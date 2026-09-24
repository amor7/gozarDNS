// Vite empties the output folders; put back the placeholders that keep
// `go build` working in a checkout where the web apps were never built.
const fs = require("fs");
const path = require("path");
for (const app of ["admin", "portal"]) {
  const dir = path.join(__dirname, "..", "..", "internal", "web", "dist", app);
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, ".keep"), "placeholder\n");
}
