#!/bin/sh

echo "Running database migrations..."
npx prisma migrate deploy

if [ $? -ne 0 ]; then
    echo "Migration failed"
    exit 1
fi

echo "Starting application..."
node server.js
