# swcat

A simple tool to define and visualize applications, their modules, processes, and interfaces.

## Getting started

### Prequisites

* Make sure you have a recent version of [Go](https://go.dev/) installed (>= 1.24.5 will do).
* Install `npm` (e.g. via [nvm](https://github.com/nvm-sh/nvm)).

### Build and run

Build the frontend artifacts:

```bash
cd web
npm install
npm run build
cd ..
```

Now run the server, using the example catalog files:

```bash
go run ./cmd/swcat -addr localhost:9191 
```

Point your browser at <http://localhost:9191> and explore the example catalog.
