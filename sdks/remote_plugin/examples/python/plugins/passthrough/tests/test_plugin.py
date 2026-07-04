import pytest

from voce.core import PluginTester
from voce.plugins.passthrough.plugin import PassthroughConfig, PassthroughPlugin
from voce.schema import Payload, Signal


@pytest.mark.asyncio
async def test_passthrough_plugin():
    # 1. Initialize Plugin
    plugin = PassthroughPlugin(PassthroughConfig())

    # 2. Setup Variables to capture results
    received_payloads = []
    received_signals = []

    # 3. Harness the extension using PluginTester
    tester = PluginTester(plugin)

    def on_payload(port: int, payload: Payload) -> None:
        received_payloads.append((port, payload))

    def on_signal(port: int, signal: Signal) -> None:
        received_signals.append((port, signal))

    tester.on_payload(on_payload).on_signal(on_signal)

    # 5. Start the lifecycle
    await tester.start()

    # 6. Simulate streaming data
    test_payload = Payload.from_json_bytes("test_payload", b'{"key": "value"}')
    await tester.inject_payload(test_payload)

    test_signal = Signal.from_json_bytes("test_signal", b'{"action": "stop"}')
    await tester.inject_signal(test_signal)

    # 7. Wait for activity to settle
    await tester.wait(0.1)

    # 8. Assert end state
    assert len(received_payloads) == 1
    assert received_payloads[0][0] == 0
    assert received_payloads[0][1].name == "test_payload"
    assert received_payloads[0][1].properties == {"key": "value"}

    assert len(received_signals) == 1
    assert received_signals[0][0] == 0
    assert received_signals[0][1].name == "test_signal"
    assert received_signals[0][1].properties == {"action": "stop"}

    # 9. Resource Cleanup
    await tester.stop()


@pytest.mark.asyncio
async def test_passthrough_plugin_with_config():
    plugin = PassthroughPlugin(
        PassthroughConfig(
            payload_prefix="in_",
            payload_suffix="_out",
            forward_signals=False,
            output_port=2,
        )
    )

    received_payloads = []
    received_signals = []

    tester = PluginTester(plugin)
    tester.on_payload(lambda port, payload: received_payloads.append((port, payload)))
    tester.on_signal(lambda port, signal: received_signals.append((port, signal)))

    await tester.start()

    await tester.inject_payload(Payload.from_json_bytes("test_payload", b'{"key": "value"}'))
    await tester.inject_signal(Signal.from_json_bytes("test_signal", b'{"action": "stop"}'))
    await tester.wait(0.1)

    assert len(received_payloads) == 1
    assert received_payloads[0][0] == 2
    assert received_payloads[0][1].name == "in_test_payload_out"
    assert received_payloads[0][1].properties == {"key": "value"}
    assert received_signals == []

    await tester.stop()
