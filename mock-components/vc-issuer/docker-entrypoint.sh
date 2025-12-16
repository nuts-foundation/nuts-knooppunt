#!/bin/sh

echo "Running database migrations..."
node ./node_modules/prisma/build/index.js db push --accept-data-loss

if [ $? -ne 0 ]; then
    echo "Migration failed"
    exit 1
fi

echo "Starting application..."
node server.js
