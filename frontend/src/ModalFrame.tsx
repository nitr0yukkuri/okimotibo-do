import type { MouseEventHandler, ReactNode } from "react";

interface ModalFrameProps {
  backdropClassName: string;
  contentClassName: string;
  labelledBy: string;
  children: ReactNode;
  onBackdropClick?: MouseEventHandler<HTMLDivElement>;
  onBackdropMouseDown?: MouseEventHandler<HTMLDivElement>;
  onContentMouseDown?: MouseEventHandler<HTMLElement>;
}

export function ModalFrame({
  backdropClassName,
  contentClassName,
  labelledBy,
  children,
  onBackdropClick,
  onBackdropMouseDown,
  onContentMouseDown,
}: ModalFrameProps) {
  return (
    <div
      className={`modal-backdrop ${backdropClassName}`}
      role="presentation"
      onClick={onBackdropClick}
      onMouseDown={onBackdropMouseDown}
    >
      <section
        className={`modal-surface ${contentClassName}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        onMouseDown={onContentMouseDown}
      >
        {children}
      </section>
    </div>
  );
}
