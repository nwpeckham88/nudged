<script lang="ts">
	import type { Agent } from '$lib/types';
	import { onMount } from 'svelte';

	let agents: Agent[] = [];
	let loading = true;
	let error: string | null = null;

	async function fetchAgents() {
		try {
			const res = await fetch('/agents');
			if (!res.ok) throw new Error('Failed to fetch agents');
			agents = await res.json();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Unknown error';
		} finally {
			loading = false;
		}
	}

	async function wakeApp(app: string) {
		if (!confirm(`Wake up ${app}?`)) return;
		try {
			const res = await fetch(`/wake?app=${encodeURIComponent(app)}`, { method: 'POST' });
			if (res.ok) {
				alert(`Wake signal sent to ${app}`);
			} else {
				alert(`Failed to wake ${app}: ${res.statusText}`);
			}
		} catch (e) {
			alert(`Error waking ${app}: ${e}`);
		}
	}

	onMount(() => {
		fetchAgents();
		const interval = setInterval(fetchAgents, 5000);
		return () => clearInterval(interval);
	});
</script>

<div class="container mx-auto p-4">
	<h1 class="mb-4 text-2xl font-bold">Nudged Hub</h1>

	{#if loading && agents.length === 0}
		<p>Loading agents...</p>
	{:else if error}
		<p class="text-red-500">Error: {error}</p>
	{:else}
		<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
			{#each agents as agent}
				<div class="rounded-lg border p-4 shadow-sm">
					<h2 class="text-xl font-semibold">{agent.name}</h2>
					<p class="text-sm text-gray-500">{agent.id}</p>
					<div class="mt-2">
						<span class="font-medium">Address:</span> {agent.addr}
					</div>
					<div class="mt-2">
						<span class="font-medium">Last Seen:</span> {new Date(agent.last_seen * 1000).toLocaleString()}
					</div>
					<div class="mt-2">
						<span class="font-medium">Apps:</span>
						{#if agent.apps && agent.apps.length > 0}
							<ul class="list-disc pl-5">
								{#each agent.apps as app}
									<li class="flex items-center justify-between mb-1">
										<span>{app}</span>
										<button
											class="ml-2 rounded bg-blue-500 px-2 py-1 text-xs text-white hover:bg-blue-600 cursor-pointer"
											onclick={() => wakeApp(app)}
										>
											Wake
										</button>
									</li>
								{/each}
							</ul>
						{:else}
							<span class="text-gray-400">None</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
		
		{#if agents.length === 0}
			<p class="text-gray-500">No agents connected.</p>
		{/if}
	{/if}
</div>
