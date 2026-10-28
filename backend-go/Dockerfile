FROM golang:1.26-alpine AS build
WORKDIR /src
COPY backend-go/go.mod backend-go/go.sum ./
RUN go mod download
COPY backend-go/ .
RUN CGO_ENABLED=0 go build -o /accountamily-go .

FROM alpine:3.20
RUN apk add --no-cache tzdata
ENV TZ=Asia/Jakarta
WORKDIR /app
COPY --from=build /accountamily-go /app/accountamily-go
COPY react/dist /app/react_dist
RUN mkdir -p /app/data
CMD ["/app/accountamily-go"]
