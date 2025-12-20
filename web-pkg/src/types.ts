// Matches the API interface defined in Go (or the wasm_bridge.go specificaly)
export interface GoApi {
  initialize(repoPath: string): Promise<void>;
  clone(repoUrl: string, repoPath: string): Promise<void>;
  addEvent(eventJson: string): Promise<void>;
  updateEvent(eventJson: string): Promise<void>;
  removeEvent(eventJson: string): Promise<void>;
  getEvent(id: number): Promise<string>;
  getEvents(from: number, to: number): Promise<string>;
}

declare global {
  interface Window {
    api: GoApi;
  }
}
