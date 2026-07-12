import assert from "node:assert/strict";
import test from "node:test";
import { parseXSourceText } from "../public/x-source-format.js";

test("X source formatting separates repost context, body, quote, and engagement tail", () => {
  const parsed = parseXSourceText({
    author: "Aaron Epstein @aaron_epstein · 23h",
    text: "Y Combinator reposted Aaron Epstein @aaron_epstein · 23h Designing your dream home just got even faster Quote Drafted @DraftedAI · Jul 10 Today, we are releasing Drafted V2. Choose the rooms you want. 18 43 1.1K 382K 383K",
  });

  assert.equal(parsed.socialContext, "Y Combinator reposted");
  assert.equal(parsed.body, "Designing your dream home just got even faster");
  assert.equal(parsed.quote.identity, "Drafted @DraftedAI · Jul 10");
  assert.equal(parsed.quote.body, "Today, we are releasing Drafted V2. Choose the rooms you want.");
});

test("ordinary X posts remain a single body", () => {
  assert.deepEqual(parseXSourceText({
    author: "Ada @ada · 2h",
    text: "Ada @ada · 2h A normal post without a quote 2 4 100",
  }), {
    socialContext: null,
    body: "A normal post without a quote",
    quote: null,
  });
});
