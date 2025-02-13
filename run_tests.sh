#!/bin/bash
set -ex

docker compose up --build -d

sleep 15

docker compose exec go_app go test -count=1 -timeout 30s -v ./tests

docker compose down
