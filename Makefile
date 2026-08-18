.PHONY: up down build api worker test clean logs

up:
	docker compose up --build -d

down:
	docker compose down

build:
	go build ./...

api:
	go run ./api/main.go

worker:
	go run ./worker/main.go

test:
	go test ./...

logs:
	docker compose logs -f --tail=100

clean:
	docker compose down -v --remove-orphans