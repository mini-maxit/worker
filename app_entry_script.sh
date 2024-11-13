#!/bin/bash

./wait-for-it.sh $RABBITMQ_HOST:$RABBITMQ_PORT

# Run the worker
exec sudo ./bin/worker-service
