#!/bin/bash

# 找到脚本的 PID
pids=$(ps aux | grep test-mr.sh | grep -v grep | awk '{print $2}')

# 检查是否找到了 PID
if [ -z "$pids" ]; then
  echo "未找到运行中的 test-mr.sh 脚本"
  exit 1
fi

# 停止脚本
for pid in $pids; do
  echo "停止脚本 PID: $pid"
  kill $pid
done

echo "脚本已停止"