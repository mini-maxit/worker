#!/bin/bash

# Wait for RabbitMQ to be available
echo $RABBITMQ_PORT
./wait-for-it.sh rabbitmq:$RABBITMQ_PORT --

# Start the worker service
exec /worker-service
