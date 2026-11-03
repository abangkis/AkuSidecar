import test from "node:test";
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { readFrontendAppSource } from "./fixtures/frontend-source.mjs";

test("frontend source contracts read the same actual declarations after LF, CRLF or mixed checkout", async t => {
  const web = new URL("../internal/httpapi/web/", import.meta.url);
  const app = readFileSync(new URL("app.js", web), "utf8");
  const files = ["app.js", ...new Set([...app.matchAll(/\/\/ Frontend feature: ([\w-]+\.js)#/g)].map(match => match[1]))];
  const expected = readFrontendAppSource();
  const build = fileURLToPath(new URL("../build/frontend-source-tests/", import.meta.url));
  mkdirSync(build, { recursive: true });
  for (const ending of ["LF", "CRLF", "mixed"]) {
    await t.test(ending, () => {
      const directory = mkdtempSync(join(build, "checkout-"));
      try {
        for (const file of files) {
          const lines = readFileSync(new URL(file, web), "utf8").replaceAll("\r\n", "\n").split("\n");
          const source = lines.map((line, index) => line + (index === lines.length - 1 ? "" : ending === "CRLF" || ending === "mixed" && index % 2 ? "\r\n" : "\n")).join("");
          writeFileSync(join(directory, file), source);
        }
        assert.equal(readFrontendAppSource(pathToFileURL(directory + sep)), expected);
      } finally {
        const target = relative(build, directory);
        assert.ok(target && !target.startsWith("..") && !target.includes(sep));
        rmSync(directory, { recursive: true, force: true });
      }
    });
  }
});
