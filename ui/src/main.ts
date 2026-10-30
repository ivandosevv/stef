import { mount } from 'svelte';
import App from './App.svelte';
import './app.css';
import { applyTheme, readTheme } from './theme';

// Restore colors before Svelte mounts to avoid flashing the default appearance.
applyTheme(readTheme());
mount(App, { target: document.getElementById('app')! });
