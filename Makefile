build:
	go build -o bin/arkstorm main.go
	cp -r fonts bin/

run:
	go run main.go "$(config)"

clean:
	rm -rf bin
	rm -rf assets
	rm -rf videos

docker:
	docker build -t arkstorm .