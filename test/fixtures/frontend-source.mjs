import { readFileSync } from "node:fs";

// Existing source/VM contracts inspect the actual feature declarations in their
// original order. Browser integration separately executes the real module graph.
export function readFrontendAppSource(web = new URL("../../internal/httpapi/web/", import.meta.url)) {
  let source = readFileSync(new URL("app.js", web), "utf8").replaceAll("\r\n", "\n");
  const files = [...new Set([...source.matchAll(/\/\/ Frontend feature: ([\w-]+\.js)#/g)].map(match => match[1]))];
  const featureImports = files.map(file => {
    const header = readFileSync(new URL(file, web), "utf8").replaceAll("\r\n", "\n").split("export function ")[0];
    const canonical = [...header.matchAll(/import \{ ([^}]+) \} from ("[^"]+");/g)].flatMap(match => match[1].split(",").map(name => `import { ${name.trim()} } from ${match[2]};`)).join("\n");
    return header + canonical + "\n";
  }).join("\n");
  const getters = new Map([...source.matchAll(/\b(get[A-Z]\w*): \(\) => (\w+),/g)].map(match => [match[1], match[2]]));
  source = source.replace(/\/\/ Frontend composition imports: begin[\s\S]*?\/\/ Frontend composition imports: end\r?\n/g, "")
    .replace(/\/\/ Frontend composition bindings: begin[\s\S]*?\/\/ Frontend composition bindings: end\r?\n/g, "");
  const declarations = source.replace(/\/\/ Frontend feature: ([\w-]+\.js)#(\w+)/g, (_, file, name) => {
    const moduleSource = readFileSync(new URL(file, web), "utf8").replaceAll("\r\n", "\n");
    const heads = [...moduleSource.matchAll(/^  (?:async )?function (\w+)\(/gm)];
    const index = heads.findIndex(match => match[1] === name);
    let declaration;
    if (index >= 0) {
      const end = index + 1 < heads.length ? heads[index + 1].index : moduleSource.lastIndexOf("\n  return {\n");
      declaration = moduleSource.slice(heads[index].index, end).trimEnd().split("\n").map(line => line.startsWith("  ") ? line.slice(2) : line).join("\n");
    } else {
      const match = moduleSource.match(new RegExp(`^  ((?:const|let) ${name} = [^\\n]+)`, "m"));
      if (!match) throw new Error(`Missing feature declaration ${file}#${name}`);
      declaration = match[1];
    }
    for (const [getter, binding] of getters) {
      declaration = declaration.replaceAll(`${getter}()`, binding);
      declaration = declaration.replace(new RegExp(`set${getter.slice(3)}\\((true|false)\\)`, "g"), `${binding} = $1`);
    }
    return declaration;
  });
  return featureImports + declarations;
}

export function frontendFunction(source, name, nextName) {
  const candidates = [`async function ${name}(`, `function ${name}(`];
  const start = candidates.map(marker => source.indexOf(marker)).find(index => index >= 0);
  const end = nextName ? source.indexOf(nextName, start + 1) : source.length;
  if (start === undefined || end <= start) throw new Error(`Missing production function ${name}`);
  return source.slice(start, end);
}
