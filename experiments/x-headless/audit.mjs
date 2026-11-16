// Sanitized aggregate readback. Post bodies, authors, IDs and media URLs stay local.
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { root } from './chrome.mjs';

export function summarize(report) {
  const posts = report.posts ?? [];
  const images = posts.flatMap(p => p.media ?? []).filter(m => m.kind === 'image');
  const videos = posts.flatMap(p => p.media ?? []).filter(m => m.kind === 'video_poster');
  const responses = report.mediaResponses ?? [];
  const httpGood = r => r.status >= 200 && r.status < 300;
  const dates = posts.map(p => Date.parse(p.publishedAt)).filter(Number.isFinite);
  const samples = report.samples ?? [];
  const primary = posts.find(p => p.permalink === report.target);
  return {
    startedAt: report.startedAt, status: report.status, profileScope: report.profileScope,
    browser: report.browser?.product, durationMs: report.durationMs,
    posts: posts.length, uniqueIds: new Set(posts.map(p => p.id)).size,
    targetKind: new URL(report.target).pathname === '/home' ? 'home_feed' : 'post',
    targetPrimaryCaptured: new URL(report.target).pathname === '/home' ? null : posts.some(p => p.permalink === report.target),
    targetPrimaryVideoFrameReady: primary ? primary.media.some(m => m.kind === 'video_poster' && m.readyState >= 2 && m.width > 0 && m.height > 0) : null,
    targetPrimaryVideoPlaying: primary ? primary.media.some(m => m.kind === 'video_poster' && m.readyState >= 2 && m.paused === false && m.currentTime > 0) : null,
    textPosts: posts.filter(p => p.text).length, timestampPosts: dates.length,
    oldestPublishedAt: dates.length ? new Date(Math.min(...dates)).toISOString() : null,
    newestPublishedAt: dates.length ? new Date(Math.max(...dates)).toISOString() : null,
    futureTimestamps: dates.filter(d => d > Date.parse(report.startedAt) + 60000).length,
    collapsedTextPosts: posts.filter(p => p.textCollapsed).length,
    relationshipCounts: Object.fromEntries(['original', 'repost', 'quote', 'reply'].map(k => [k, posts.filter(p => p.relationshipType === k).length])),
    quotedBodiesExported: false,
    imageUrls: images.length, decodedImages: images.filter(m => m.loaded && m.width > 0 && m.height > 0).length,
    videoPosters: videos.length, videoMetadataReady: videos.filter(m => m.readyState >= 1).length,
    videoFramesReady: videos.filter(m => m.readyState >= 2 && m.width > 0 && m.height > 0).length,
    videoPlayingObserved: videos.filter(m => m.readyState >= 2 && m.paused === false && m.currentTime > 0).length,
    blobVideoSources: videos.filter(m => m.sourceKind === 'blob').length,
    posterHttpVerified: videos.filter(m => responses.some(r => r.url === m.url && httpGood(r) && r.mimeType.startsWith('image/'))).length,
    postsExpectingImageWithoutImageUrl: posts.filter(p => p.mediaExpected?.includes('image') && !p.media?.some(m => m.kind === 'image')).length,
    postsExpectingVideoWithoutPoster: posts.filter(p => p.mediaExpected?.includes('video') && !p.media?.some(m => m.kind === 'video_poster')).length,
    mediaHttpResponses: responses.length, mediaHttpErrors: responses.filter(r => r.status >= 400).length,
    videoHttpResponses: responses.filter(r => /^video\/|mpegurl/i.test(r.mimeType)).length,
    videoResponsesWithBytes: responses.filter(r => /^video\/|mpegurl/i.test(r.mimeType) && httpGood(r) && r.encodedBytes > 0).length,
    duplicateObservations: report.duplicateObservations ?? null,
    rejectedObservations: samples.reduce((n, s) => n + s.rejected, 0),
    rejectionReasons: Object.fromEntries(['missing_identity', 'ambiguous_identity', 'missing_author', 'empty_evidence']
      .map(k => [k, samples.reduce((n, s) => n + (s.rejectionReasons?.[k] ?? 0), 0)])),
    authenticatedEverySample: samples.length ? samples.every(s => s.authenticatedUiObserved) : null,
    challengeObserved: samples.some(s => s.challengeDetected),
    maxScrollY: samples.length ? Math.max(...samples.map(s => s.scroll?.y ?? 0)) : null,
    samplingRounds: samples.length, stopReason: report.stopReason,
    progression: samples.map(s => ({ round: s.round, newPosts: s.newPosts, total: s.uniquePosts, y: s.scroll?.y })),
  };
}

export async function auditLatest(count = 1) {
  const folders = (await readdir(resolve(root, 'runs'), { withFileTypes: true }))
    .filter(f => f.isDirectory()).map(f => f.name).sort().reverse();
  const records = [];
  for (const folder of folders) {
    const report = JSON.parse(await readFile(resolve(root, 'runs', folder, 'capture.json'), 'utf8'));
    if ((report.source || 'x') !== 'x' || report.status !== 'captured_partial' || !report.samples?.[0]?.scroll) continue;
    const summary = summarize(report);
    await writeFile(resolve(root, 'runs', folder, 'summary.json'), JSON.stringify(summary, null, 2) + '\n');
    records.push(summary);
    if (records.length >= count) break;
  }
  return records;
}
if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const count = Number(process.argv[2] ?? 1);
  if (!Number.isInteger(count) || count < 1 || count > 10) throw new Error('Audit count must be 1..10.');
  console.log(JSON.stringify(await auditLatest(count), null, 2));
}
