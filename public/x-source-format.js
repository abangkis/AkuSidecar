export function parseXSourceText(candidate = {}) {
  let text = String(candidate.text ?? "").replace(/^Feed post\s+/i, "").trim();
  const author = String(candidate.author ?? "").trim();
  let socialContext = null;

  if (author) {
    const authorIndex = text.indexOf(author);
    if (authorIndex >= 0) {
      socialContext = text.slice(0, authorIndex).trim() || null;
      text = text.slice(authorIndex + author.length).trim();
    }
  }

  text = stripEngagementTail(text);
  const quoteMarker = text.indexOf(" Quote ");
  if (quoteMarker < 0) {
    return { socialContext, body: text, quote: null };
  }

  const body = text.slice(0, quoteMarker).trim();
  const quoted = text.slice(quoteMarker + " Quote ".length).trim();
  const header = quoted.match(/^(.+?\s+@\w+\s+·\s+(?:[A-Z][a-z]{2}\s+\d{1,2}|\d+[smhdw]))\s+([\s\S]+)$/);
  return {
    socialContext,
    body,
    quote: header
      ? { identity: header[1].trim(), body: header[2].trim() }
      : { identity: "Quoted post", body: quoted },
  };
}

function stripEngagementTail(value) {
  return value.replace(/(?:\s+\d+(?:[.,]\d+)?[KMB]?){3,6}$/i, "").trim();
}
