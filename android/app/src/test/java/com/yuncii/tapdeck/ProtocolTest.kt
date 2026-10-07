package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test
import java.nio.ByteBuffer
import java.nio.ByteOrder

class ProtocolTest {
    @Test fun matchesGoEncryptedVector() {
        val codec = UdpCodec(ByteArray(32) { it.toByte() }, byteArrayOf(0x11, 0x22, 0x33, 0x44), 0x0102030405060708, 1)
        repeat(9) { codec.seal(ByteArray(36)) }
        val actual = codec.seal(Movement(3, 1024, -2048, 0, 122880).bytes()).joinToString("") { "%02x".format(it.toInt() and 255) }
        assertEquals("54444b310100000008070605040302010900000000000000733488da86b890b41259a3809e84f94ad41b8662462290bac51fca3471e832104297707dd4debe4b3dc28aaaf5a7aaf549e21408", actual)
    }
    @Test fun nonceDoesNotResetWithNewRecording() {
        val codec = UdpCodec(ByteArray(32), byteArrayOf(1, 2, 3, 4), 1, 2)
        val a = codec.seal(ByteBuffer.allocate(976).order(ByteOrder.LITTLE_ENDIAN).putLong(100).array())
        val b = codec.seal(ByteBuffer.allocate(976).order(ByteOrder.LITTLE_ENDIAN).putLong(200).array())
        assertEquals(0, ByteBuffer.wrap(a).order(ByteOrder.LITTLE_ENDIAN).getLong(16))
        assertEquals(1, ByteBuffer.wrap(b).order(ByteOrder.LITTLE_ENDIAN).getLong(16))
    }
    @Test fun configKeepsEnabledOriginalSlotNumbers() {
        val c = PcConfig(revision = 2, shortcuts = (0..7).map { Shortcut("键 $it", "F${it + 1}", it == 0 || it == 3 || it == 7) }, voice = Voice(hold_key = "RightAlt")).validate()
        val encoded = wireJson.encodeToJsonElement(PcConfig.serializer(), c).toString().dropLast(1) + ""","gain":1}"""
        val restored = wireJson.decodeFromString<PcConfig>(encoded).validate()
        assertEquals(listOf(0, 3, 7), restored.visibleShortcuts().map { it.index })
        assertEquals("RightAlt", restored.voice.hold_key)
        assertEquals(2, restored.revision)
        assertEquals(8, restored.shortcuts.size)
    }
    @Test(expected = IllegalArgumentException::class) fun emptyEnabledShortcutsRejected() { PcConfig(shortcuts = List(8) { Shortcut("A", "F1", false) }).validate() }
}
