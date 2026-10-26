# Imagem única: o React é compilado e embutido no binário do Go.

# 1) Telas
FROM node:22-alpine AS telas
WORKDIR /telas
COPY frontend/package*.json ./
RUN npm install --no-audit --no-fund
COPY frontend/ .
RUN npm run build

# 2) Servidor
FROM golang:1.24-alpine AS servidor
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=telas /telas/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /estuda-junto ./cmd/servidor

# 3) Imagem final mínima (sem shell, sem pacotes, usuário sem privilégios)
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=servidor /estuda-junto /estuda-junto
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/estuda-junto"]
