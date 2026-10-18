import test from "node:test";
import assert from "node:assert/strict";
import { buildPostScreenshotHTML } from "../internal/httpapi/web/post-screenshot-render.js";

const image = "data:image/png;base64,iVBORw0KGgo=";

test("post screenshot preserves full escaped text, quote ownership, identity, date, and mapped raster previews", () => {
  const entry = {
    source: "linkedin",
    item: { author: "Item author", publishedAt: "2026-10-18T10:00:00Z", sourceUrl: "https://private.example/post" },
    evidence: {
      author: "<Author & name>",
      publishedAt: "2026-10-18T09:30:00Z",
      avatarUrl: "https://media.example/avatar.jpg",
      text: "Full post line 1\n\n</p><script>steal()</script> & keep formatting.",
      presentation: { headline: "Researcher <one>", socialContext: "Reposted by <colleague>", timestampText: "2h" },
      attachments: [
        { title: "<Report>", subtitle: "PDF & notes", domain: "example.test", imageUrl: "https://media.example/report.png", url: "https://private.example/file" },
        { title: "Unavailable report", imageUrl: "https://media.example/missing.png" },
      ],
      media: [{ kind: "image", url: "https://media.example/post.jpg", alt: "Post <image>", width: 1200, height: 800 }],
      quotedPost: {
        author: "Quote author",
        publishedAt: "2026-10-17T10:00:00Z",
        text: "Quoted full text with <unsafe> markup.",
        media: [{ kind: "video", url: "https://media.example/clip.mp4", posterUrl: "https://media.example/poster.jpg", width: 640, height: 360 }],
      },
      engagement: { like: 12 },
    },
  };
  const media = {
    "https://media.example/avatar.jpg": image,
    "https://media.example/report.png": image,
    "https://media.example/post.jpg": image,
    "https://media.example/poster.jpg": image,
  };
  const { html, warnings } = buildPostScreenshotHTML(entry, { css: ".source-layout-card { color: red; }", theme: "light", media });

  assert.deepEqual(warnings, []);
  assert.match(html, /data-post-screenshot-theme="light"/);
  assert.match(html, /data-post-screenshot-export-root="true"/);
  assert.match(html, /data-post-screenshot-export-height="document"/);
  assert.match(html, /http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'/);
  assert.match(html, /<strong>&lt;Author &amp; name&gt;<\/strong>/);
  assert.ok(html.includes("Full post line 1\n\n&lt;/p&gt;&lt;script&gt;steal()&lt;/script&gt; &amp; keep formatting."));
  assert.match(html, /Quoted full text with &lt;unsafe&gt; markup\./);
  assert.match(html, /source-layout-media-item is-video-poster/);
  assert.match(html, /Video poster/);
  assert.match(html, /width="1200" height="800"/);
  assert.match(html, /Reposted by &lt;colleague&gt;/);
  assert.equal((html.match(/src="data:image\/png;base64,/g) || []).length, 4);
  assert.doesNotMatch(html, /https:\/\/media\.example/);
  assert.doesNotMatch(html, /href=/);
  assert.doesNotMatch(html, /<script(?:\s|>)/i);
  assert.doesNotMatch(html, /<video(?:\s|>)/i);
  assert.doesNotMatch(html, /private\.example/);
  assert.match(html, /&lt;Report&gt;/);
  assert.match(html, /Attachment image preview unavailable/);
});

test("all captured media entries retain vertical order and unavailable or unsafe media gets a visible placeholder", () => {
  const media = Array.from({ length: 24 }, (_, index) => ({
    kind: "image",
    url: `https://media.example/${index}.jpg`,
    alt: `image ${index}`,
  }));
  const entry = {
    source: "x",
    item: { whatChanged: "Summary should not replace captured text" },
    evidence: {
      text: "Captured text",
      media: [...media, { kind: "image", url: "https://media.example/unsafe.svg", alt: "unsafe mapped type" }],
    },
  };
  const mapped = Object.fromEntries(media.map((value) => [value.url, image]));
  mapped["https://media.example/unsafe.svg"] = "data:image/svg+xml;base64,PHN2Zz4=";
  const result = buildPostScreenshotHTML(entry, { media: mapped });

  assert.deepEqual(result.warnings, []);
  assert.match(result.html, /data-post-screenshot-media-count="25"/);
  assert.equal((result.html.match(/class="source-layout-media-item"/g) || []).length, 25);
  assert.equal((result.html.match(/class="post-screenshot-media-unavailable"/g) || []).length, 1);
  assert.ok(result.html.indexOf("image 0") < result.html.indexOf("image 23"));
  assert.match(result.html, /Captured text/);
  assert.doesNotMatch(result.html, /Summary should not replace captured text/);
  assert.doesNotMatch(result.html, /whatChanged/);
});

test("renderer escapes stylesheet termination and rejects untrusted source URLs as media", () => {
  const result = buildPostScreenshotHTML({
    source: "x\"><img src=x onerror=alert(1)>",
    evidence: { text: "safe", media: [{ url: "javascript:alert(1)" }] },
  }, {
    css: "p::after { content: '</style><script>alert(1)</script>'; }",
    media: { "javascript:alert(1)": "data:image/png;base64,iVBORw0KGgo=" },
  });

  assert.match(result.html, /source-unknown/);
  assert.ok(result.html.includes("\\3C /style>\\3C script>alert(1)\\3C /script>"));
  assert.match(result.html, /Image preview unavailable/);
  assert.doesNotMatch(result.html, /src="javascript:/i);
  assert.doesNotMatch(result.html, /<script(?:\s|>)/i);
  assert.doesNotMatch(result.html, /<img src=x onerror=/i);
});
