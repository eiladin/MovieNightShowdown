import { useEffect } from 'react'
import confettiPkg from 'canvas-confetti'

const confetti = typeof confettiPkg === 'function' ? confettiPkg : (confettiPkg as any).default || confettiPkg

export default function Confetti() {
  useEffect(() => {
    const duration = 3000
    const end = Date.now() + duration

    const interval = setInterval(() => {
      if (Date.now() > end) {
        clearInterval(interval)
        return
      }
      try {
        confetti({
          particleCount: 20,
          angle: 60,
          spread: 55,
          origin: { x: 0 },
          colors: ['#26ccff', '#a25afd', '#ff5e7e', '#88ff5a', '#fcff42', '#ffa62d', '#ff36ff'],
          zIndex: 1001
        })
        confetti({
          particleCount: 20,
          angle: 120,
          spread: 55,
          origin: { x: 1 },
          colors: ['#26ccff', '#a25afd', '#ff5e7e', '#88ff5a', '#fcff42', '#ffa62d', '#ff36ff'],
          zIndex: 1001
        })
      } catch (e) {
        console.error('Confetti error:', e)
        clearInterval(interval)
      }
    }, 250)

    return () => clearInterval(interval)
  }, [])

  return null
}
