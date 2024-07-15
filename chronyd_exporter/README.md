# TODO
1. 添加flag，可以指定端口
2. 添加flag，展示版本信息

# 说明
展示 chronyd 的状态。当前版本使用 `:9105` 端口暴漏指标内容，访问 `http://ip:9105/metrics` 可访问指标内容。

# 使用
## 方式一：
直接在后台运行
```bash
nohup ./chronyd_exporter &
```

## 方式二：
systemd 管理（未验证，后续更新）。

# 指标内容
```
# HELP chronyd_leap_status Chronyd leap status (0 = Normal, 1 = Insert second, 2 = Delete second, 3 = Not synchronised, 4 = Unkonw)
# TYPE chronyd_leap_status gauge
chronyd_leap_status 0
# HELP chronyd_status Chronyd status (1 = running, 0 = not running)
# TYPE chronyd_status gauge
chronyd_status 1
# HELP current_time Current host time in Unix timestamp
# TYPE current_time gauge
current_time 1.721008074e+09
```