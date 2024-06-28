package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
	_ "github.com/lib/pq"
)

type IndexInfo struct {
	SchemaName string `json:"schema_name"`
	IndexName  string `json:"index_name"`
}

// DBConfig 用于存储数据库连接信息
type DBConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
	SSLMode  string `json:"sslmode"`
}

func main() {
	// 命令行参数
	version := "0.0.1"
	var printVersion bool
	configFile := flag.String("config", "./conf/config.json", "Path to the database config file")
	numWorkers := flag.Int("workers", 10, "Number of concurrent workers")
	flag.BoolVar(&printVersion, "version", false, "print program build version")
	flag.Parse()

	if printVersion {
		fmt.Printf("Reindex Tool: %s\n", version)
		os.Exit(0)
	}

	// 加载数据库配置
	config, err := loadDBConfig(*configFile)
	if err != nil {
		log.Fatal(err)
	}

	// 日志设置
	logFilePath := fmt.Sprintf("./logs/reindex_index_%s_%s.log", config.Host, config.Port)
	logger, err := initLogger(logFilePath)
	if err != nil {
		log.Fatal(err)
	}

	// 连接到数据库
	db, err := connectDB(config)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	fmt.Println("Successfully connected to the postgres database")
	logger.Println("Successfully connected to the postgres database")

	// 获取业务数据库名称
	businessDatabases, err := fetchBusinessDatabases(db)
	if err != nil {
		log.Fatal(err)
	}
	db.Close()

	// 使用 WaitGroup 等待所有 worker 完成
	var wg sync.WaitGroup

	// 设置并发的 worker 数量
	// numWorkers := 10

	// 创建进度条
	totalTasks := 0

	// 对每个业务数据库进行连接并获取索引信息
	for _, dbName := range businessDatabases {
		fmt.Printf("Start reindex on database %s\n", dbName)
		config.DBName = dbName
		db, err := connectDB(config)
		if err != nil {
			log.Fatal(err)
		}

		logger.Printf("Connected to business database: %s\n", dbName)

		indexes, err := fetchIndexesInfo(db)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		// 如果没有索引，跳过这个数据库
		if len(indexes) == 0 {
			db.Close()
			logger.Printf("No indexes found in database: %s, skipping...\n", dbName)
			continue
		}

		// 将生成的 SQL 语句添加到 sqlStatements 列表，并更新 totalTasks
		sqlStatements := make(chan string, len(indexes))
		results := make(chan string, len(indexes))
		for _, index := range indexes {
			sqlStmt := fmt.Sprintf("REINDEX INDEX \"%s\".\"%s\"", index.SchemaName, index.IndexName)
			sqlStatements <- sqlStmt
			totalTasks++
		}
		close(sqlStatements)

		// 创建进度条
		progress := pb.StartNew(len(indexes))

		// 启动 worker
		for w := 1; w <= *numWorkers; w++ {
			wg.Add(1)
			go worker(w, db, sqlStatements, results, &wg, logger, dbName)
		}

		// 启动一个 goroutine 来等待所有 worker 完成并关闭 results 通道
		go func() {
			wg.Wait()
			close(results)
		}()

		// 更新进度条
		for range results {
			progress.Increment()
		}
		progress.Finish()

		fmt.Printf("database %s complate\n", dbName)
	}
	logger.Println("All tasks completed")
	fmt.Println("All tasks completed")
}

// worker 是一个并发执行 SQL 语句的工作函数
func worker(id int, db *sql.DB, jobs <-chan string, results chan<- string, wg *sync.WaitGroup, logger *log.Logger, dbNmae string) {
	defer wg.Done()
	for sqlStmt := range jobs {

		start := time.Now()
		_, err := db.Exec(sqlStmt)
		duration := time.Since(start)

		if err != nil {
			// log.Printf("Worker %d: Error executing %s: %v", id, sqlStmt, err)
			result := fmt.Sprintf("Worker %d: database: %s, Error executing: %s: %v", id, dbNmae, sqlStmt, err)
			logger.Println(result)
			results <- result
		} else {
			result := fmt.Sprintf("Worker %d: database: %s, Successfully executed: %s (Duration: %v)", id, dbNmae, sqlStmt, duration)
			logger.Println(result)
			results <- result
		}
	}
}

// loadDBConfig 从 JSON 文件加载数据库连接配置
func loadDBConfig(filePath string) (*DBConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var config DBConfig
	if err := json.Unmarshal(byteValue, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// connectDB 使用提供的配置连接到数据库
func connectDB(config *DBConfig) (*sql.DB, error) {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(20) // 设置最大打开连接数，根据需求调整
	db.SetMaxIdleConns(10) // 设置最大空闲连接数，根据需求调整

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// initLogger 初始化日志文件和 logger
func initLogger(logFilePath string) (*log.Logger, error) {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	// 创建一个新的 logger，输出到日志文件
	logger := log.New(logFile, "", log.LstdFlags)
	return logger, nil
}

// fetchIndexesInfo 从数据库中获取索引信息
func fetchIndexesInfo(db *sql.DB) ([]IndexInfo, error) {
	var indexes []IndexInfo

	indexesQuery := `
        SELECT schemaname, indexname
        FROM pg_indexes
        WHERE schemaname != 'pg_catalog' AND schemaname != 'information_schema';
    `
	rows, err := db.Query(indexesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var schemaName, indexName string
		if err := rows.Scan(&schemaName, &indexName); err != nil {
			return nil, err
		}
		indexes = append(indexes, IndexInfo{SchemaName: schemaName, IndexName: indexName})
	}

	return indexes, nil
}

// fetchBusinessDatabases 从数据库中获取业务数据库名称
func fetchBusinessDatabases(db *sql.DB) ([]string, error) {
	var dbNames []string

	dbsQuery := `SELECT datname FROM pg_database WHERE datistemplate = false AND datname NOT IN ('postgres', 'template0', 'template1', 'zcloud');`
	rows, err := db.Query(dbsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			return nil, err
		}
		dbNames = append(dbNames, dbName)
	}

	return dbNames, nil
}
