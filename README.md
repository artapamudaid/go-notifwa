# Go-Notifwa

WhatsApp Gateway written in Go using [Fiber](https://gofiber.io/) and [Whatsmeow](https://pkg.go.dev/go.mau.fi/whatsmeow). This service is designed to be a high-performance backend, connecting with the Laravel backend.

## Requirements
- Go 1.20+
- MySQL Server

## Setup
1. Clone this repository.
2. Copy `.env.example` to `.env` and adjust the database credentials.
3. Install dependencies:
   ```bash
   go mod tidy
   ```
4. Run the server:
   ```bash
   go run main.go
   ```

The server will start at `http://localhost:3001` (or whatever is defined in your `.env` or main.go, defaults to `8088`).

---

## High-Performance Worker Queue
This gateway implements a **Multi-Worker Pool Queue**. 
When the API receives a `POST` request to send a message, media, or poll, it does **not** block to wait for WhatsApp to send it. Instead, it places the job in a background channel and immediately returns an HTTP success response. 

This allows you to send **blasts to thousands of numbers concurrently** without experiencing HTTP timeouts from your Laravel / PHP backend. The background workers will then process the queue safely, applying a random 1-5 second delay per message to prevent bans, while memory references are automatically cleared (Garbage Collected) after each send.

---

## Docker Deployment

You can also deploy this application using Docker.

### 1. Build the Image

```bash
docker build -t go-notifwa .
```

### 2. Run the Container

Make sure you have an `.env` file ready. Also, create an empty `examplestore.db` file to persist your WhatsApp sessions if you don't have one yet.

> **Memory GC Optimization**: The Dockerfile automatically injects `ENV GOMEMLIMIT=250MiB` to optimize Go's Garbage Collector. This ensures that the worker queue aggressively clears old memory references, preventing memory leaks and keeping the container's RAM usage strictly below 250MB.

```bash
touch examplestore.db
docker run -d \
  --name go-notifwa \
  -p 8088:8088 \
  -v $(pwd)/.env:/app/.env \
  -v $(pwd)/examplestore.db:/app/examplestore.db \
  --restart unless-stopped \
  go-notifwa
```

> **Important note on Database Connection**: If your MySQL database is running on your host machine (not in Docker), the container cannot connect to it using `localhost` or `127.0.0.1` as `DB_HOST` in your `.env` file. You need to change `DB_HOST` to `host.docker.internal` in your `.env` file for the container to reach the host's database.

---

## API Documentation

All endpoints expect data encoded as `application/x-www-form-urlencoded` or `application/json`.

### 1. Send Text Message
**Endpoint:** `POST /backend-send-text`

| Parameter | Type   | Description |
|-----------|--------|-------------|
| `token`   | string | Session / Device token used for authentication. |
| `number`  | string | Destination phone number (e.g. `0812xxx` or `62812xxx`) or Group JID. |
| `text`    | string | The message content. |

**Response (Success):**
```json
{
  "status": true,
  "message": "Message queued successfully"
}
```

### 2. Send Media Message
**Endpoint:** `POST /backend-send-media`

| Parameter  | Type   | Description |
|------------|--------|-------------|
| `token`    | string | Session / Device token. |
| `number`   | string | Destination phone number or Group JID. |
| `url`      | string | URL to download the media file. |
| `type`     | string | Media type: `image`, `video`, `audio`, `document`. |
| `caption`  | string | (Optional) Caption for image/video/document. |
| `filename` | string | (Optional) Filename for document. |

### 3. Send Poll Message
**Endpoint:** `POST /backend-send-poll`

| Parameter   | Type    | Description |
|-------------|---------|-------------|
| `token`     | string  | Session / Device token. |
| `number`    | string  | Destination phone number or Group JID. |
| `name`      | string  | The title/question of the poll. |
| `options`   | string  | JSON array string for poll options (e.g. `["Yes", "No"]`). |
| `countable` | boolean | Set `1` or `true` if multiple selections are allowed. |

### 4. Get Connected Groups
**Endpoint:** `POST /backend-getgroups`

| Parameter | Type   | Description |
|-----------|--------|-------------|
| `token`   | string | Session / Device token. |

**Response (Success):**
Returns a list of groups the device has joined.
```json
{
  "status": true,
  "data": [
    {
      "id": "12345678@g.us",
      "name": "My Group",
      "subject": "My Group",
      "participants": [
         { "id": "628xxx@s.whatsapp.net" }
      ]
    }
  ]
}
```

### 5. WebSocket Connection for QR & Status
**Endpoint:** `WS /ws/connect/:device`

Used by the frontend to connect and scan the QR code.
- `:device` represents the session token (e.g. `mydevice`).
