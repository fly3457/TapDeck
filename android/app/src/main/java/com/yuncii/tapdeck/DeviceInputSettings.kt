package com.yuncii.tapdeck

import java.util.Locale
import kotlin.math.round
import kotlinx.coroutines.*
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Local device preferences, independent of the connected computer's configuration. */
data class DeviceInputSettings(val sensitivity: Double = 1.0, val haptics: Boolean = true) {
    companion object {
        const val BASE_GAIN = 2.0
        const val MIN = 0.5
        const val MAX = 3.0
        const val STEP = 0.1

        fun normalize(value: Double): Double =
            if (!value.isFinite()) 1.0 else round(value.coerceIn(MIN, MAX) / STEP) / 10.0
    }

    fun normalized() = copy(sensitivity = normalize(sensitivity))
    val pointerGain: Double get() = BASE_GAIN * normalize(sensitivity)
    val sensitivityLabel: String get() = String.format(Locale.ROOT, "%.1f×", normalize(sensitivity))
}

/** One ordered writer: a late load cannot overwrite an edit, nor can older writes win. */
internal class DeviceInputPreferences(
    load: suspend () -> DeviceInputSettings,
    save: suspend (DeviceInputSettings) -> Unit,
    onError: (Throwable) -> Unit = {},
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val lock = Any()
    private var pendingEdits: MutableList<(DeviceInputSettings) -> DeviceInputSettings>? = mutableListOf()
    private val changes = Channel<Unit>(Channel.CONFLATED)
    private val mutable = MutableStateFlow(DeviceInputSettings())
    val state = mutable.asStateFlow()
    private val writer = scope.launch {
        try {
            try {
                val saved = load().normalized()
                synchronized(lock) {
                    val edits = pendingEdits.orEmpty()
                    val merged = edits.fold(saved) { value, change -> change(value).normalized() }
                    mutable.value = merged
                    pendingEdits = null
                }
            } catch (e: Exception) {
                if (e is CancellationException) throw e
                synchronized(lock) { pendingEdits = null }
                onError(e)
            }
            for (ignored in changes) {
                val value = synchronized(lock) { mutable.value }
                try { save(value) }
                catch (e: Exception) { if (e is CancellationException) throw e; onError(e) }
            }
        } finally { scope.cancel() }
    }

    fun update(change: (DeviceInputSettings) -> DeviceInputSettings) = synchronized(lock) {
        pendingEdits?.add(change)
        val next = change(mutable.value).normalized()
        mutable.value = next
        changes.trySend(Unit)
        Unit
    }

    // Finishes the last queued save even when the Activity/ViewModel has been destroyed.
    fun close() { changes.close() }
    suspend fun awaitClosed() { writer.join() }
}
