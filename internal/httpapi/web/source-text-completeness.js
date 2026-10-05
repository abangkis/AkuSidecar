// Completeness comes from capture evidence, independently of local UI folding.
export function sourceTextIsPartial(quality) {
  const status = quality?.textStatus || "";
  const limitations = Array.isArray(quality?.limitations) ? quality.limitations : [];
  return ["requires_permalink_capture", "visible_text_may_be_collapsed", "expand_failed",
    "expansion_skipped_detect_only", "navigation_changed"].includes(status)
    || limitations.some(value => ["text_may_be_collapsed", "text_may_be_truncated", "text_truncated"].includes(value));
}
