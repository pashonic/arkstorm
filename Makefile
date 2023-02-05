build:
	go build -o bin/arkstorm main.go

run:
	go run main.go "$(config)"

test:
	go test ./...

clean:
	rm -rf bin
	rm -rf assets
	rm -rf videos

docker:
	docker build -t arkstorm .