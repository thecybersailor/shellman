import http from "node:http";

const port = Number(process.env.WORKER_PORT ?? "8787");

function json(res, status, body) {
  res.writeHead(status, {
    "content-type": "application/json; charset=utf-8",
    "cache-control": "no-store"
  });
  res.end(JSON.stringify(body));
}

const server = http.createServer((req, res) => {
  const method = req.method ?? "GET";
  const url = req.url ?? "/";

  if (method === "POST" && url === "/api/register") {
    json(res, 200, {
      turn_uuid: "mock-turn-e2e",
      visit_url: "http://127.0.0.1/t/mock-turn-e2e",
      agent_ws_url: "ws://127.0.0.1/ws/agent/mock-turn-e2e"
    });
    return;
  }

  if (method === "GET" && (url === "/" || url === "/healthz")) {
    json(res, 200, { ok: true, mode: "mock-worker" });
    return;
  }

  json(res, 404, { error: "not_found", path: url });
});

server.listen(port, "0.0.0.0", () => {
  // Keep output plain for log grep/readability.
  process.stdout.write(`mock worker listening on :${port}\n`);
});
