# --- Build frontend ---
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Build backend ---
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /out/showdown .

# --- Runtime ---
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/showdown /showdown
EXPOSE 8080
ENTRYPOINT ["/showdown"]
