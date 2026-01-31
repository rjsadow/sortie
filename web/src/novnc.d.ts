declare module '@novnc/novnc/lib/rfb.js' {
  interface RFBOptions {
    credentials?: { password?: string };
    shared?: boolean;
    wsProtocols?: string[];
  }

  class RFB {
    constructor(target: HTMLElement, url: string, options?: RFBOptions);
    disconnect(): void;
    scaleViewport: boolean;
    resizeSession: boolean;
    viewOnly: boolean;
    clipViewport: boolean;
    addEventListener(event: string, handler: (e: unknown) => void): void;
    removeEventListener(event: string, handler: (e: unknown) => void): void;
  }

  export default RFB;
}
