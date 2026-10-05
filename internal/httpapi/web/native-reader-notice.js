export function nativeReaderNotice(status, { busy = false, feedback = "" } = {}) {
  const blocked = status?.nativeReaderBlocked === true;
  const since = new Date(status?.nativeReaderBlockedSince ?? "");
  const sinceText = Number.isFinite(since.getTime())
    ? ` Tertunda sejak ${since.toLocaleString("id-ID", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}.`
    : "";
  return {
    visible: blocked || busy || Boolean(feedback),
    disabled: busy || !blocked,
    title: busy ? "Menutup jendela post dan menyiapkan update…"
      : blocked ? "Update tertunda karena jendela post masih terbuka." : "Jendela post sudah ditutup.",
    detail: `Menutup semua jendela post yang dibuka AkuBrowser.${sinceText}`,
    button: busy ? "Menutup jendela post…" : "Tutup jendela post & lanjutkan update",
    feedback,
  };
}
