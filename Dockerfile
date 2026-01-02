# ---- build ----
FROM golang:1.22 AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/frameserve ./main.go

# ---- runtime ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/frameserve /frameserve

EXPOSE 80
USER nonroot:nonroot
ENTRYPOINT ["/frameserve"]
