.PHONY: run test build

run:
	go run .

test:
	go test -v ./...

build:
	go build -o bin/bobot .
	GOOS=linux GOARCH=arm64 go build -o bin/bobot-arm64 .

deploy: build
	rsync -avzP bin/bobot-arm64 $(BOBOT_SSH_SERVER):$(BOBOT_SSH_SERVER_PATH)/bobot-next
	ssh $(BOBOT_SSH_SERVER) "systemctl stop bobot && mv -f $(BOBOT_SSH_SERVER_PATH)/bobot-next $(BOBOT_SSH_SERVER_PATH)/bobot && systemctl start bobot"

logs:
	ssh $(BOBOT_SSH_SERVER) "journalctl -u bobot -u bobot-update-profiles -f"
