clear
go build -buildmode=plugin ../mrapps/wc.go
find . -maxdepth 1 -type f -name 'mr-*' -exec rm -f {} \;
go run mrworker.go wc.so