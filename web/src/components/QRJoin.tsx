import QRCode from 'react-qr-code'

interface QRJoinProps {
  joinURL: string
}

// QRJoin renders a scannable join link for guests to point their phone
// camera at, shown to the admin in the Lobby.
export default function QRJoin({ joinURL }: QRJoinProps) {
  return (
    <div className="qr-join">
      <QRCode value={joinURL} size={180} />
      <p className="qr-join-url">{joinURL}</p>
    </div>
  )
}
