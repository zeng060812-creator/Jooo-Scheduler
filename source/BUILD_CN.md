Jooo Scheduler / jood Android ARM64 构建

要求：Go 1.23+

cd source/jood
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test ./...
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ../../bin/jood .

当前平台层不依赖厂商私有SDK：标准/proc、/sys cpufreq、Android dumpsys、taskset。
