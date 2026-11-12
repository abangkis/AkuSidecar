export function renderExecutableSelection(input, path, mode = "") {
  input.value = path || "";
  input.dataset.executableBaseline = input.value.trim();
  input.dataset.executableMode = mode;
}

export function readExecutableSelection(input) {
  const path = input.value.trim();
  const edited = path !== input.dataset.executableBaseline;
  return {
    reasoningExecutablePath: path,
    reasoningExecutableMode: edited ? (path ? "explicit" : "automatic") : input.dataset.executableMode || "",
  };
}
