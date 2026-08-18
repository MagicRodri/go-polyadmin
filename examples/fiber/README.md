# examples/fiber

Reference Fiber application exercising `go/polyadmin`, mirroring
`examples/fastapi`: `User` and `Organization` models with a foreign
key relation, a dashboard, search/filter/sort, actions, and CSV export.

Run it:

```bash
go run .
# open http://127.0.0.1:3000/admin
```

See [`../../go/README.md`](../../go/README.md) for the Go adapter's
known gaps relative to the Python one (no XLSX export, no per-resource
template override).
