import { useEffect, useState } from "react";
import QRCode from "qrcode";
import { buildPairingUrl, pairingApiBase, type PairingSession } from "./pairing";
import type { StatusSyncOptions } from "./use-status-sync";
import { ModalFrame } from "./ModalFrame";

interface PairingQrModalProps {
  sync?: StatusSyncOptions;
  roomLoading?: boolean;
  onClose: () => void;
}

interface PairingResponse extends PairingSession {
  code: string;
  expiresAt: string;
}

const copy = {
  title: "スマホ接続",
  missingConfig: "ルーム接続設定がないため、接続情報を発行できません。",
  issueFailed: "スマホ接続情報を発行できませんでした。",
  invalid: "接続情報が不正です。",
  failed: "QR発行に失敗しました。",
  alt: "スマホ接続用QRコード",
  note: "スマホでQRを読み取るか、下の合言葉を入力してください。",
  expiry: "このQRと合言葉は10分間だけ有効です。合言葉は1回だけ使えます。",
  loadingRoom: "ルーム接続設定を取得しています…",
  loading: "接続情報を発行しています…",
};

export function PairingQrModal({ sync, roomLoading = false, onClose }: PairingQrModalProps) {
  const [pairing, setPairing] = useState<PairingSession>();
  const [pairingCode, setPairingCode] = useState<string>();
  const [qrDataUrl, setQrDataUrl] = useState<string>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    let cancelled = false;
    setPairing(undefined);
    setPairingCode(undefined);
    setQrDataUrl(undefined);
    setError(undefined);
    if (!sync) {
      if (!roomLoading) setError(copy.missingConfig);
      return () => { cancelled = true; };
    }

    const issuePairing = async () => {
      try {
        const response = await fetch(`${pairingApiBase(sync.url)}/api/v1/pairing`, {
          method: "POST",
          headers: {
            ...(sync.token ? { Authorization: `Bearer ${sync.token}` } : {}),
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            roomId: sync.roomId,
            ...(sync.token ? {} : { userId: sync.userId }),
          }),
        });
        if (!response.ok) throw new Error(copy.issueFailed);
        const data = await response.json() as Partial<PairingResponse>;
        if (!data.token || !data.code || !data.roomId || !data.userId) throw new Error(copy.invalid);
        const nextPairing: PairingSession = { token: data.token, roomId: data.roomId, userId: data.userId };
        const image = await QRCode.toDataURL(buildPairingUrl(nextPairing), {
          errorCorrectionLevel: "M",
          margin: 2,
          width: 260,
          color: { dark: "#232323", light: "#ffffff" },
        });
        if (cancelled) return;
        setPairing(nextPairing);
        setPairingCode(data.code);
        setQrDataUrl(image);
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : copy.failed);
      }
    };

    void issuePairing();
    return () => { cancelled = true; };
  }, [roomLoading, sync?.roomId, sync?.token, sync?.url, sync?.userId]);

  return (
    <ModalFrame
      backdropClassName="modal-overlay"
      contentClassName="pairing-modal"
      labelledBy="pairing-modal-title"
      onBackdropClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <button className="modal-close-button" type="button" aria-label="スマホ接続を閉じる" onClick={onClose}>
        ×
      </button>
      <h3 id="pairing-modal-title">{copy.title}</h3>
      {qrDataUrl && pairingCode ? (
        <>
          <img className="pairing-qr" src={qrDataUrl} alt={copy.alt} />
          <p className="pairing-modal-note">{copy.note}</p>
          <p className="pairing-code" aria-label={`合言葉 ${pairingCode}`}>{pairingCode}</p>
          {pairing && <p className="pairing-modal-expiry">{copy.expiry}</p>}
        </>
      ) : error ? (
        <p className="pairing-modal-error" role="alert">{error}</p>
      ) : (
        <p className="pairing-modal-note">{roomLoading ? copy.loadingRoom : copy.loading}</p>
      )}
    </ModalFrame>
  );
}
