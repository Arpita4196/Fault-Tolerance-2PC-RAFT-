FROM golang:1.24-alpine
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY proto ./proto
COPY services/query-api/ .
RUN go mod tidy
RUN go build -o query-api .
EXPOSE 50052
CMD ["./query-api"]
