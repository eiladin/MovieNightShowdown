import QRCode from 'react-qr-code'

interface QRJoinProps {
  joinURL: string
}

// QRJoin renders a scannable join link for guests to point their phone
// camera at, shown to the admin in the Lobby. The white padded wrapper
// provides the "quiet zone" the QR spec requires for reliable scanning.
export default function QRJoin({ joinURL }: QRJoinProps) {
  return (
    <div className="qr-join">
      <div className="qr-join-code">
        <QRCode value={joinURL} size={220} level="M" style={{ width: '100%', height: 'auto', display: 'block' }} />
      </div>
      <p className="qr-join-url">{joinURL}</p>
    </div>
  )
}
