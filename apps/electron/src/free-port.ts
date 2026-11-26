// A free TCP port on loopback, asked of the system: the cluster app's daemons each take one.
import net from 'node:net';

export const freePort = (): Promise<number> =>
  new Promise((resolve, reject) => {
    const s = net.createServer().once('error', reject);
    s.listen(0, '127.0.0.1', () => {
      const { port } = s.address() as net.AddressInfo;
      s.close(() => resolve(port));
    });
  });
