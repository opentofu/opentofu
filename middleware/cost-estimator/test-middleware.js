#!/usr/bin/env node

const { spawn } = require('child_process');

// Start the middleware process
const middleware = spawn('opentofu-cost-estimator', [], {
  stdio: ['pipe', 'pipe', 'pipe']
});

// Test messages
const messages = [
  {
    jsonrpc: "2.0",
    method: "initialize",
    params: {
      capabilities: ["pre-plan", "post-plan", "pre-apply", "post-apply"]
    },
    id: 1
  },
  {
    jsonrpc: "2.0",
    method: "post-plan",
    params: {
      resource_type: "aws_s3_bucket",
      resource_name: "test_bucket",
      provider: "aws",
      planned_action: "Create",
      config: {
        bucket: "test-bucket-123",
        tags: {
          EstimatedStorageGB: "100",
          EstimatedMonthlyGETRequests: "50000"
        }
      }
    },
    id: 2
  },
  {
    jsonrpc: "2.0",
    method: "shutdown",
    params: {},
    id: 3
  }
];

let messageIndex = 0;

middleware.stdout.on('data', (data) => {
  console.log('Response:', data.toString());
  
  // Send next message after receiving response
  if (messageIndex < messages.length) {
    setTimeout(() => {
      const message = JSON.stringify(messages[messageIndex]) + '\n';
      console.log('Sending:', message.trim());
      middleware.stdin.write(message);
      messageIndex++;
    }, 100);
  } else {
    middleware.kill();
  }
});

middleware.stderr.on('data', (data) => {
  console.error('Error:', data.toString());
});

middleware.on('close', (code) => {
  console.log(`Middleware exited with code ${code}`);
});

// Send first message
const firstMessage = JSON.stringify(messages[0]) + '\n';
console.log('Sending:', firstMessage.trim());
middleware.stdin.write(firstMessage);
messageIndex++;