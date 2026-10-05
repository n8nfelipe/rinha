FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/records-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/records-api /records-api
EXPOSE 8080
ENTRYPOINT ["/records-api"]
