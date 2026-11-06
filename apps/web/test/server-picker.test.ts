import { expect, it } from 'vitest';
import { serverUrl } from '#src/tools/ServerPicker.tsx';

it('opens a server at the address it listens on, localhost for this machine or every interface', () => {
  expect(['127.0.0.1', '0.0.0.0', '::', '::1', '100.1.2.3'].map((address) => serverUrl({ port: 5173, address }))).toEqual([
    'http://localhost:5173',
    'http://localhost:5173',
    'http://localhost:5173',
    'http://[::1]:5173',
    'http://100.1.2.3:5173',
  ]);
});
