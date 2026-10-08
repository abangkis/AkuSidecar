export function nativeReaderNotice(status, { busy = false, feedback = "", runtimeFailure = "" } = {}) {
  const blocked = status?.nativeReaderBlocked === true;
  const requiresManualClose = [feedback, runtimeFailure].some(value => /close(?: it)? manually/i.test(value));
  const manualActionRequired = blocked && !busy && requiresManualClose;
  const displayedFeedback = requiresManualClose
    ? blocked && !busy ? "Jendela ini berisi tab yang tidak dapat ditutup otomatis. Tutup jendelanya langsung di browser agar update dapat dilanjutkan." : ""
    : feedback;
  const since = new Date(status?.nativeReaderBlockedSince ?? "");
  const sinceText = Number.isFinite(since.getTime())
    ? ` Tertunda sejak ${since.toLocaleString("id-ID", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}.`
    : "";
  return {
    visible: blocked || busy || Boolean(displayedFeedback),
    disabled: busy || !blocked,
    title: busy ? "Menutup jendela post dan menyiapkan update…"
      : blocked ? "Update tertunda karena jendela post masih terbuka." : "Jendela post sudah ditutup.",
    detail: `Menutup semua jendela post yang dibuka AkuBrowser.${sinceText}`,
    button: busy ? "Menutup jendela post…" : "Tutup jendela post & lanjutkan auto update",
    feedback: displayedFeedback,
    manualAction: manualActionRequired ? "Anda perlu menutup jendela post secara manual." : "",
  };
}

export function nativeReaderResumeMessage(response) {
  if (response.session) return "Jendela post ditutup. Auto Update mulai menyiapkan batch.";
  const reason = response.resumeReason || response.autoUpdate?.reason;
  if (reason === "Bounded generation allowance reached") {
    const next = new Date(response.autoUpdate?.nextCheckAt ?? "");
    const nextText = Number.isFinite(next.getTime())
      ? ` Pemeriksaan berikutnya sekitar ${next.toLocaleTimeString("id-ID", { hour: "2-digit", minute: "2-digit" })}.`
      : " Scheduler akan memeriksa kembali secara otomatis.";
    return `Jendela post ditutup. Auto update menunggu batas frekuensi.${nextText}`;
  }
  return `Jendela post ditutup. ${reason || "Status Auto Update sudah diperbarui."}`;
}
