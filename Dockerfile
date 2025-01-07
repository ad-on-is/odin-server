FROM golang:alpine AS bebuilder
WORKDIR /build/
COPY ./apps/backend/go.mod .
COPY ./apps/backend/go.sum .
RUN CGO_ENABLED=0 GOOS=linux go mod download
COPY ./apps/backend .
RUN CGO_ENABLED=0 GOOS=linux go build -o odin-backend

FROM node:alpine AS fe-builder
WORKDIR /build/
RUN npm i -g pnpm
COPY ./apps/frontend/package.json . 
RUN pnpm i
COPY ./apps/frontend .
RUN pnpm run build


FROM node:alpine
RUN apk --update add ca-certificates curl mailcap caddy mosquitto bash

COPY --from=fe-builder /build/.output /odin-frontend
COPY --from=bebuilder /build/odin-backend /odin-server
COPY ./apps/services/Caddyfile /etc/caddy/Caddyfile
COPY ./apps/services/mosquitto.conf /etc/mosquitto/mosquitto.conf

COPY ./apps/backend/migrations/1735493885_collections_snapshot.go /migrations/1735493885_collections_snapshot.go

COPY ./entrypoint.sh /entrypoint.sh
COPY ./version.txt /version.txt

CMD ["/entrypoint.sh"]


