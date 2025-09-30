# swcat

A simple tool to define and visualize applications, their modules, processes, and interfaces.

## Getting started

Make sure you have a recent version of Go installed (>= 1.24.5 will do).

Build the frontend artifacts:

```bash
cd web
npm install
npm run build
```

Now run the server, using the example catalog files:

```bash
go run ./cmd/swcat -addr localhost:9191 examples/twosys/
```

Point your browser at <http://localhost:9191> and explore the example catalog.
