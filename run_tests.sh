#!/bin/bash
set -ex

docker compose -f docker-compose.test.yml up --build -d

sleep 15

docker compose exec go_app go test -count=1 -timeout 100s -v ./tests

docker compose down
