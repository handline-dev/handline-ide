// A video a comment carries, played in a window: from its thumbnail in the chat or on a sub-task's Result tab.
import { useAttachmentUrl } from '@handline/app-data';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';

export function playVideo(id: string, name: string) {
  void openModal<void>((close) => (
    <ModalFrame title={name} size="xl" onClose={() => close()}>
      <Player id={id} />
    </ModalFrame>
  ));
}

function Player({ id }: { id: string }) {
  const { url, failed } = useAttachmentUrl(id, false);
  if (failed) return <p className="pb-4 text-sm text-danger">The video did not load.</p>;
  return url ? (
    <video src={url} controls autoPlay className="mb-4 w-full rounded-md bg-fg" />
  ) : (
    <div className="mb-4 aspect-video rounded-md bg-hover/30" />
  );
}
