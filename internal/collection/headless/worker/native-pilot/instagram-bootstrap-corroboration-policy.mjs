// Acquisition, stream retirement (at most 1s), and target cleanup fit inside
// the existing 30s Go RPC deadline with a 2s serialization/drain reserve.
export const INSTAGRAM_CORROBORATION_POLICY=Object.freeze({maxBodyBytes:4*1024*1024,chunkBytes:65536,maxHeadersBytes:65536,maxHeaders:256,totalMS:30000,acquisitionMS:24000,cleanupMS:3000,freshMS:6000,evaluationMS:4000});
export const INSTAGRAM_CORROBORATION_HOME='https://www.instagram.com/';
export const INSTAGRAM_CORROBORATION_READY="location.origin==='https://www.instagram.com'&&location.pathname==='/'&&['interactive','complete'].includes(document.readyState)";
