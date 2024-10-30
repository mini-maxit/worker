#!/bin/bash

./wait-for-it.sh $QUEUE_BROKER:$RABBITMQ_PORT --

# Run the worker
exec ./bin/worker-service
