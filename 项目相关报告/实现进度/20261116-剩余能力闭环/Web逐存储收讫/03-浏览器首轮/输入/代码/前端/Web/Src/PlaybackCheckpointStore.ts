// Created by Jobs on 2026年11月17日，星期二.
import { storageDatabase } from "./StorageDatabase";
import { knownGlobalIDs, readGlobalFacts } from "./GlobalMessages";
import { checkpointMatches, nextCheckpoint, playbackScope, pruneCheckpointEntries, transitionCheckpoint, validPlaybackBinding, type PlaybackBinding, type PlaybackCheckpoint } from "./PlaybackCheckpointProtocol";

// Only metadata lives here. Every read/write shares the authoritative global and auth fences.
async function transact(binding: PlaybackBinding, isCurrent: () => boolean, action: "read" | "save" | "forget", next?: PlaybackCheckpoint): Promise<PlaybackCheckpoint | undefined> {
  if (!validPlaybackBinding(binding) || !isCurrent()) throw Error("PLAYBACK_CONTEXT_CHANGED");
  const db = await storageDatabase;
  if (!isCurrent()) throw Error("PLAYBACK_CONTEXT_CHANGED");
  return new Promise((resolve, reject) => {
    const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), store = tx.objectStore("spaces"), key = "playback-checkpoints|" + binding.namespace;
    const active = store.get("auth-active|" + playbackScope(binding.namespace)), fence = store.get("auth-frozen|" + binding.namespace), facts = readGlobalFacts(store, binding.namespace)!, read = store.get(key);
    let result: PlaybackCheckpoint | undefined, failure: unknown;
    read.onsuccess = () => {
      try {
        if (!isCurrent() || active.result !== binding.namespace || fence.result) throw Error("PLAYBACK_CONTEXT_CHANGED");
        const globals = knownGlobalIDs(binding.namespace, facts.result);
        let holder = pruneCheckpointEntries(read.result, globals);
        if (!globals.has(binding.messageId)) holder = transitionCheckpoint(holder, binding, action, next);
        result = holder.entries.find(entry => checkpointMatches(entry, binding));
        if (!isCurrent()) throw Error("PLAYBACK_CONTEXT_CHANGED");
        store.put(holder, key);
      } catch (error) { failure = error; tx.abort(); }
    };
    tx.oncomplete = () => isCurrent() ? resolve(result) : reject(Error("PLAYBACK_CONTEXT_CHANGED"));
    tx.onerror = () => reject(failure ?? tx.error ?? Error("PLAYBACK_STORAGE_FAILED"));
    tx.onabort = () => reject(failure ?? tx.error ?? Error("PLAYBACK_STORAGE_FAILED"));
  });
}
export const loadPlaybackCheckpoint = (binding: PlaybackBinding, isCurrent: () => boolean) => transact(binding, isCurrent, "read");
export const forgetPlaybackCheckpoint = (binding: PlaybackBinding, isCurrent: () => boolean) => transact(binding, isCurrent, "forget");
export const savePlaybackCheckpoint = (binding: PlaybackBinding, position: number, duration: number, heard: boolean, isCurrent: () => boolean) => transact(binding, isCurrent, "save", nextCheckpoint(binding, position, duration, heard, Date.now()));
