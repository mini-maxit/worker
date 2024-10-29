#!/bin/bash

# Wait for RabbitMQ to be available
./wait-for-it.sh rabbitmq:$RABBITMQ_PORT --

# Run the worker service
exec /app/bin/worker-service
