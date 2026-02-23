# Project Manager Backend API Contract

## Scope

This document defines backend contracts for Project Manager multi-session chat in overview mode.

## Endpoints

### List sessions

- Method: `GET`
- Path: `/api/v1/projects/{project_id}/pm/sessions`
- Response:

```json
{
  "ok": true,
  "data": {
    "project_id": "p1",
    "items": [
      {
        "session_id": "uuid",
        "project_id": "p1",
        "title": "PM Session",
        "archived": false,
        "last_message_at": 1700000000000,
        "created_at": 1700000000000,
        "updated_at": 1700000000000
      }
    ]
  }
}
```

### Create session

- Method: `POST`
- Path: `/api/v1/projects/{project_id}/pm/sessions`
- Request:

```json
{
  "title": "PM Session"
}
```

- Response:

```json
{
  "ok": true,
  "data": {
    "session_id": "uuid",
    "project_id": "p1",
    "title": "PM Session"
  }
}
```

### Send message

- Method: `POST`
- Path: `/api/v1/projects/{project_id}/pm/sessions/{session_id}/messages`
- Request:

```json
{
  "content": "hello",
  "source": "user_input"
}
```

- Response:

```json
{
  "ok": true,
  "data": {
    "project_id": "p1",
    "session_id": "uuid",
    "status": "queued",
    "source": "user_input"
  }
}
```

## Runtime Behavior

- Message processing is serialized per `session_id` using actor model.
- Different sessions can run in parallel.
- User and assistant messages are persisted in `pm_messages`.
- Assistant message status lifecycle: `running -> completed|failed`.

## Error Codes

- `PROJECT_NOT_FOUND`
- `PM_SESSION_NOT_FOUND`
- `INVALID_MESSAGE`
- `AGENT_LOOP_UNAVAILABLE`
- `PM_MESSAGE_ENQUEUE_FAILED`

## Events

- Topic: `project.pm.messages.updated`
- Payload keys:
  - `session_id`
  - `source`
  - optional `error`
