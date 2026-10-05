FROM golang:1.22-alpine AS build

WORKDIR /src
COPY main.go .
COPY web ./web
RUN GO111MODULE=off go build -o /runner .

FROM maven:3.9-eclipse-temurin-17

WORKDIR /workspace
COPY --from=build /runner /runner
EXPOSE 8090
ENTRYPOINT ["/runner"]
