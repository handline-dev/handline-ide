// The three release channels of the Mac app (.github/workflows/mac-release.yml): each is a rolling release at its own tag, and the address a build
// reads its updates from ends in that tag. A build's own channel is the one its address names; the person can pick another one in Settings.
import { UPDATE_CHANNELS, type UpdateChannel } from '@handline/app-core';

export type { UpdateChannel };

export const isChannel = (v: unknown): v is UpdateChannel => UPDATE_CHANNELS.includes(v as UpdateChannel);

const TAG = new RegExp(`mac-(${UPDATE_CHANNELS.join('|')})/*$`);

/** The channel a build's update address names, null for an address that is none of the three. */
export function channelOf(base: string): UpdateChannel | null {
  const m = TAG.exec(base);
  return m ? (m[1] as UpdateChannel) : null;
}

/** The same address on another channel's release; an address that names no channel stays as it is. */
export const withChannel = (base: string, channel: UpdateChannel): string => base.replace(TAG, `mac-${channel}`);
