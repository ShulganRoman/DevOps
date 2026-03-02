const http = require("http");

const host = process.env.HOST || "0.0.0.0";
const port = Number(process.env.PORT || process.env.APP_PORT || 3000);
const appName = process.env.APP_NAME || "lab4-node-app";
const appMessage = process.env.APP_MESSAGE || "Hello from the Docker lab";

const server = http.createServer((request, response) => {
  const body = {
    appName,
    appMessage,
    method: request.method,
    url: request.url,
    timestamp: new Date().toISOString()
  };

  response.writeHead(200, { "Content-Type": "application/json; charset=utf-8" });
  response.end(JSON.stringify(body, null, 2));
});

server.listen(port, host, () => {
  console.log(`${appName} is listening on http://${host}:${port}`);
});
