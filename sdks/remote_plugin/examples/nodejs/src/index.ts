import { App, pluginRegistry } from '@voce/plugin-sdk'
import { captionPlugin } from './caption.js'

async function main() {
  pluginRegistry.register(captionPlugin)

  // Configure the remote plugin server on a standard port
  const app = new App({
    host: '127.0.0.1',
    port: 50052,
  })

  // Start the server
  await app.start()

  // Handle graceful shutdown on Ctrl+C
  process.on('SIGINT', async () => {
    console.log('\nReceived SIGINT, shutting down...')
    await app.stop()
    process.exit(0)
  })
}

main().catch((err) => {
  console.error('Failed to start application:', err)
  process.exit(1)
})
