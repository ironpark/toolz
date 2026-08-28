# Persist settings

Define defaults, merge persisted values during plugin loading, and await writes.

```ts
interface MySettings { enabled: boolean }
const DEFAULT_SETTINGS: MySettings = { enabled: true };

async onload() {
	this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData() as Partial<MySettings>);
	await this.saveData(this.settings);
}
```
