#!/bin/bash

# Wait for PostgreSQL to be available
./wait-for-it.sh postgres:5432 -- 

# Wait for RabbitMQ to be available
./wait-for-it.sh rabbitmq:5672 -- 

# Start the worker service
exec /worker-service
