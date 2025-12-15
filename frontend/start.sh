#!/bin/bash

# Quick start script for Peerster Crypto Wallet Frontend

echo "🚀 Starting Peerster Crypto Wallet Frontend"
echo ""

# Check if node_modules exists
if [ ! -d "node_modules" ]; then
    echo "📦 Installing dependencies..."
    npm install
fi

echo "🔧 Starting development server..."
echo "   Frontend will be available at: http://localhost:5173"
echo "   Make sure your Go backend is running on: http://localhost:8080"
echo ""

npm run dev
