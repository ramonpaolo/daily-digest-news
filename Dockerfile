FROM golang:1.26.5-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/daily-digest-news ./cmd/daily-digest-news

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/daily-digest-news /daily-digest-news
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/daily-digest-news", "serve"]
