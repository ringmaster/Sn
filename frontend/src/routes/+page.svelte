<script lang="ts">
	import { onMount } from 'svelte';
	import { dataStore, fetchData } from '../store';

	let count = 0;

	function increment() {
		count += 1;
	}
	let username = '';
	let password = '';
	let message = '';
	let loggedIn = false;

	// Fetch data when the component is mounted
	onMount(() => {
		fetchData();
	});

	async function login(event: Event) {
		event.preventDefault();
		const credentials = btoa(`${username}:${password}`);
		const response = await fetch('./api', {
			method: 'POST',
			headers: {
				Authorization: `Basic ${credentials}`
			}
		});
		if (response.ok) {
			try {
				let loginInfo = JSON.parse(await response.text());
				if (loginInfo.loggedIn) {
					message = `Logged in as ${loginInfo.username}`;
				} else {
					message = 'Login failed. Please check your credentials.';
				}
			} catch (e) {
				message = 'Login request failed.';
			}
		} else {
			message = 'Login failed. Please check your credentials.';
		}
		fetchData();
	}

	async function logout() {
		const response = await fetch('./api', {
			method: 'DELETE'
		});
		if (response.ok) {
			message = 'Logged out successfully.';
		} else {
			message = 'Logout request failed.';
		}
		fetchData();
	}
</script>

<div class="container mx-auto px-4">
	<h1 class="text-2xl font-bold mb-4 text-center">{$dataStore.title}</h1>
</div>

{#if $dataStore.loggedIn}
<div class="mb-4">
	<div class="container mx-auto px-4 text-center">
		<h2 class="text-xl font-bold mb-4 text-center">Welcome, {$dataStore.username}!</h2>
	</div>
	<button
	on:click={logout}
	class="bg-red-500 hover:bg-red-700 text-white font-bold py-2 px-4 rounded focus:outline-none focus:shadow-outline"
>
	Logout
</button>
</div>
{:else}
<form on:submit={login} class="w-full max-w-sm mx-auto mt-8">
	<div class="mb-4">
		<label class="block text-gray-700 text-sm font-bold mb-2" for="username"> Username </label>
		<input
			class="shadow appearance-none border rounded w-full py-2 px-3 text-gray-700 leading-tight focus:outline-none focus:shadow-outline"
			id="username"
			type="text"
			bind:value={username}
			required
			autocomplete="username"
		/>
	</div>
	<div class="mb-6">
		<label class="block text-gray-700 text-sm font-bold mb-2" for="password"> Password </label>
		<input
			class="shadow appearance-none border rounded w-full py-2 px-3 text-gray-700 mb-3 leading-tight focus:outline-none focus:shadow-outline"
			id="password"
			type="password"
			bind:value={password}
			required
			autocomplete="current-password"
		/>
	</div>
	<div class="flex items-center justify-between">
		<button
			class="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded focus:outline-none focus:shadow-outline"
			type="submit"
		>
			Login
		</button>
	</div>
	{#if message}
		<p class="mt-4 text-center text-red-500">{message}</p>
	{/if}
</form>
{/if}