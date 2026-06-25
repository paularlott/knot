const docs = (title, body) => `<b>${title}</b><br/>${body}`;

export const eventBodyCompletions = [
  {
    caption: "${{ .event.id }}",
    value: "${{ .event.id }}",
    meta: "event",
    score: 1000,
    docHTML: docs("Event ID", "UUIDv7 event identifier"),
  },
  {
    caption: "${{ .event.type }}",
    value: "${{ .event.type }}",
    meta: "event",
    score: 1000,
    docHTML: docs("Event Type", "Event type string"),
  },
  {
    caption: "${{ .event.ts }}",
    value: "${{ .event.ts }}",
    meta: "event",
    score: 1000,
    docHTML: docs("Event Timestamp", "HLC timestamp"),
  },
  {
    caption: "${{ json .event.data }}",
    value: "${{ json .event.data }}",
    meta: "event",
    score: 1000,
    docHTML: docs("Event Data", "Full event payload as JSON"),
  },
  {
    caption: "${{ .space.id }}",
    value: "${{ .space.id }}",
    meta: "space",
    score: 900,
    docHTML: docs("Space ID", "Source space UUID"),
  },
  {
    caption: "${{ .space.name }}",
    value: "${{ .space.name }}",
    meta: "space",
    score: 900,
    docHTML: docs("Space Name", "Source space name"),
  },
  {
    caption: "${{ json .space.urls }}",
    value: "${{ json .space.urls }}",
    meta: "space",
    score: 900,
    docHTML: docs("Space Port URLs", "All port URLs as JSON, keyed by port name (e.g. web, web2)"),
  },
  {
    caption: "${{ .actor.id }}",
    value: "${{ .actor.id }}",
    meta: "actor",
    score: 800,
    docHTML: docs("Actor ID", "User/system/MCP ID that triggered"),
  },
  {
    caption: "${{ .actor.username }}",
    value: "${{ .actor.username }}",
    meta: "actor",
    score: 800,
    docHTML: docs("Actor Username", "Username"),
  },
  {
    caption: "${{ .actor.kind }}",
    value: "${{ .actor.kind }}",
    meta: "actor",
    score: 800,
    docHTML: docs("Actor Kind", "User | System | MCP"),
  },
];
