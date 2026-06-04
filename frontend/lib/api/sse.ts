export interface ParsedSSEEvent {
  event: string;
  data: string;
}

export function parseSSEBuffer(buffer: string): { events: ParsedSSEEvent[]; rest: string } {
  const events: ParsedSSEEvent[] = [];
  let rest = buffer;
  let index = rest.indexOf("\n\n");

  while (index >= 0) {
    const raw = rest.slice(0, index);
    rest = rest.slice(index + 2);
    const parsed = parseSSEEvent(raw);
    if (parsed) {
      events.push(parsed);
    }
    index = rest.indexOf("\n\n");
  }

  return { events, rest };
}

export function parseSSEEvent(raw: string): ParsedSSEEvent | null {
  const lines = raw.split(/\r?\n/);
  let event = "message";
  const data: string[] = [];

  for (const line of lines) {
    if (line.startsWith("event:")) {
      event = line.slice(6).trim();
      continue;
    }
    if (line.startsWith("data:")) {
      data.push(line.slice(5).trimStart());
    }
  }

  if (data.length === 0) {
    return null;
  }
  return { event, data: data.join("\n") };
}
