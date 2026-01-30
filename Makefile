.PHONY: run test build

run:
	go run .

test:
	go test -v ./...

build:
	go build -o bin/bobot .
	GOOS=linux GOARCH=arm64 go build -o bin/bobot-arm64 .

deploy: build
	scp bin/bobot-arm64 root@nunes.dev:/data/bobot/bobot-next
	ssh root@nunes.dev "systemctl stop bobot && mv -f /data/bobot/bobot-next /data/bobot/bobot && systemctl start bobot"

server-logs:
	ssh root@nunes.dev "journalctl -u bobot -f"
