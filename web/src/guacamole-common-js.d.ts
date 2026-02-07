declare module 'guacamole-common-js' {
  const Guacamole: {
    Client: new (tunnel: unknown) => {
      getDisplay: () => {
        getElement: () => HTMLElement;
        getWidth: () => number;
        getHeight: () => number;
        scale: (scale: number) => void;
        onresize: (() => void) | null;
      };
      connect: (data?: string) => void;
      disconnect: () => void;
      sendMouseState: (state: unknown) => void;
      sendKeyEvent: (pressed: number, keysym: number) => void;
      onstatechange: ((state: number) => void) | null;
      onerror: ((status: { code: number; message: string }) => void) | null;
      onclipboard: ((stream: GuacamoleStream, mimetype: string) => void) | null;
    };
    WebSocketTunnel: new (url: string) => unknown;
    Mouse: new (element: HTMLElement) => {
      onmousedown: ((state: unknown) => void) | null;
      onmouseup: ((state: unknown) => void) | null;
      onmousemove: ((state: unknown) => void) | null;
    };
    Keyboard: new (element: HTMLElement | Document) => {
      onkeydown: ((keysym: number) => void) | null;
      onkeyup: ((keysym: number) => void) | null;
    };
  };

  interface GuacamoleStream {
    onblob: ((data: string) => void) | null;
    onend: (() => void) | null;
  }

  export default Guacamole;
}
