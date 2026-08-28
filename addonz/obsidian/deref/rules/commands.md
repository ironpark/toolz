# Add a command

Register commands with stable IDs through `Plugin.addCommand`.

```ts
this.addCommand({
	id: 'your-command-id',
	name: 'Do the thing',
	callback: () => this.doTheThing(),
});
```
