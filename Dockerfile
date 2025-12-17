FROM golang:1.24-alpine AS builder

RUN mkdir /src
WORKDIR /src

COPY . /src/
RUN go mod download

ENV CGO_ENABLED=0
RUN go build -o ./bin/lifeline ./cmd/bot

FROM alpine AS runtime

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /src/bin/lifeline /bin/lifeline
COPY --from=builder /src/internal/database/migrations /internal/database/migrations

ENV TZ=Asia/Taipei

CMD ["/bin/lifeline"]
