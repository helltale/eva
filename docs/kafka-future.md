# Event bus (future, post-MVP)

The technical specification (`tz-eva.json`) lists optional async events when moving to a distributed topology:

- `conversation.created`, `message.created`
- `assistant.response.completed`, `assistant.speech.started`, `assistant.speech.finished`
- `tool.started`, `tool.completed`, `tool.failed`
- `memory.updated`, `search.performed`

**Kafka** is explicitly out of scope for MVP. If you extract services (LLM gateway, search, voice orchestrator), publish these as protobuf or JSON CloudEvents on Kafka topics with consumer groups per service.

The monolith can remain event-free until you need cross-service fan-out or audit pipelines.
