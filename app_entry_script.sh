#!/bin/bash

./wait-for-it.sh $RABBITMQ_HOST:$RABBITMQ_PORT --

# Run the worker
exec ./bin/worker-service
