export const storageDatabase = new Promise<IDBDatabase>((resolve, reject) => {
  const request = indexedDB.open("JobsIM", 1);
  request.onupgradeneeded = () => request.result.createObjectStore("spaces");
  request.onsuccess = () => resolve(request.result);
  request.onerror = () => reject(request.error);
});
