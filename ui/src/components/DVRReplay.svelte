<script lang="ts">
  let { progress, playing, replaying, count, total, onplay, onseek, onlive }: {
    progress: number; playing: boolean; replaying: boolean; count: number; total: number;
    onplay: () => void; onseek: (progress: number) => void; onlive: () => void;
  } = $props();
</script>

<section class="replay-panel" aria-label="Session replay">
  <button class="replay-button" onclick={onplay} disabled={!total} aria-label={playing ? 'Pause replay' : 'Play six-second replay'}>{playing ? 'Ⅱ' : '▶'}</button>
  <div class="replay-title"><strong>Session replay</strong><span>{replaying ? 'Following the captured timeline' : 'The whole story in six seconds'}</span></div>
  <div class="scrubber"><div class="scrubber-labels"><span>{count} / {total} events</span><span>6 SEC REPLAY</span></div><input aria-label="Replay position" type="range" min="0" max="1000" step="1" value={progress * 1000} disabled={!total} oninput={(event) => onseek(Number(event.currentTarget.value) / 1000)} /></div>
  <button class="live-button" class:active={!replaying} onclick={onlive}><i></i>{replaying ? 'Return to live' : 'Live view'}</button>
</section>
