const apiBaseURL = '/hostscore/api';
var locations = ['EU', 'US'];
var hosts = [];
var offset = 0;

loadHosts();

function loadHosts() {
	document.getElementById('host').classList.add('hidden');
	let options = {
		method: 'GET',
		headers: {
			'Content-Type': 'application/json;charset=utf-8'
		}
	}
	let network = document.getElementById('select-network').value;
	hosts = [];
	offset = 0;
	let location = locations[0];
	fetch(apiBaseURL + '/hosts?location=' + location + '&network=' + network, options)
	.then(response => response.json())
	.then(data => {
		if (data.status != 'ok') console.log(data.message)
		else {
			hosts = data.hosts;
			renderHosts();
		}
	})
	.catch(error => console.log(error));
}

function renderHosts() {
	let table = document.getElementById('hosts-table');
	let all = document.getElementById('select-hosts').value == 'all';
	let nav = document.getElementById('nav');
	nav.classList.add('hidden');
	table.innerHTML = '<tr><th style="text-align:left">ID</th><th style="text-align:left">Address</th><th>Status</th></tr>';
	let filter = document.getElementById('search-host').value;
	let filteredHosts = hosts.filter(host => (all || isOnline(host.publicKey)) &&
		(filter == '' || host.netaddress.indexOf(filter) >= 0));
	for (let i = 0; i < 10; i++) {
		if (offset + i >= filteredHosts.length) break;
		let host = filteredHosts[offset + i];
		let online = isOnline(host.publicKey);
		let row = document.createElement('tr');
		row.setAttribute('key', host.publicKey);
		row.innerHTML = '<td>' + host.id + '</td>' +
			'<td><a key="' + host.publicKey + '" onclick="browseHost(this)">' +
			host.netaddress + '</a></td>' +
			'<td><div class="host-status host-status-' +
			(online && host.settings.acceptingcontracts ? 'good' :
			(online ? 'medium' : 'bad')) +
			'"></div></td>';
		table.appendChild(row);
	}
	nav.classList.remove('hidden');
	if (offset == 0) {
		document.getElementById('hosts-prev').classList.add('disabled');
	} else {
		document.getElementById('hosts-prev').classList.remove('disabled');
	}
	if (offset + 10 >= filteredHosts.length) {
		document.getElementById('hosts-next').classList.add('disabled');
	} else {
		document.getElementById('hosts-next').classList.remove('disabled');
	}
}

function isOnline(pk) {
	let host = hosts.find(h => h.publicKey == pk);
	if (host) {
		return host.scanHistory.length > 0 && host.scanHistory[0].success == true &&
			((host.scanHistory.length > 1 && host.scanHistory[1].success == true) ||
			host.scanHistory.length == 1);
	}
	return false;
}

function prevHosts() {
	offset -= 10;
	renderHosts();
}

function nextHosts() {
	offset += 10;
	renderHosts();
}

function hostSelectionChange() {
	offset = 0;
	renderHosts();
}

function browseHost(obj) {
	let key = obj.getAttribute('key');
	let host = hosts.find(h => h.publicKey == key);
	let options = {
		method: 'GET',
		headers: {
			'Content-Type': 'application/json;charset=utf-8'
		}
	}
	let network = document.getElementById('select-network').value;
	let table = document.getElementById('host-benchmarks');
	let list = document.getElementById('host-benchmarks-list');
	table.classList.add('hidden');
	list.classList.add('hidden');
	let latencies = [];
	let benchmarks = [];
	let from = new Date();
	from.setDate(from.getDate() - 1);
	locations.forEach(location => {
		fetch(apiBaseURL + '/scans?location=' + location.toLowerCase() +
			'&network=' + network +
			'&host=' + key +
			'&from=' + from.toISOString(), options)
		.then(response => response.json())
		.then(data => {
			if (data.status != 'ok') console.log(data.message)
			else {
				let average = 0;
				let count = 0;
				data.scans.forEach(scan => {
					if (scan.success == true) {
						average += scan.latency / 1e6;
						count++;
					}
				});
				if (count > 0) {
					average /= count;
					latencies.push({location: location, latency: average.toFixed(0) + ' ms'});
				} else {
					latencies.push({location: location, latency: 'N/A'});
				}
				if (latencies.length == locations.length) {
					let header = '<tr><th></th>';
					let row = '<tr><td>Latency</td>';
					locations.forEach(loc => {
						latency = latencies.find(l => l.location == loc);
						header += '<th>' + latency.location + '</th>';
						row += '<td style="text-align:center">' + latency.latency + '</td>';
					});
					header += '</tr>';
					row += '</tr>';
					table.children[1].innerHTML = header + row;
					locations.forEach(loc => {
						fetch(apiBaseURL + '/benchmarks?location=' + loc.toLowerCase() +
							'&network=' + network +
							'&host=' + key +
							'&from=' + from.toISOString(), options)
						.then(response => response.json())
						.then(data => {
							if (data.status != 'ok') console.log(data.message)
							else {
								let ul = 0;
								let dl = 0;
								let ttfb = 0;
								let count = 0;
								data.benchmarks.forEach(benchmark => {
									if (benchmark.success == true) {
										ul += benchmark.uploadSpeed;
										dl += benchmark.downloadSpeed;
										ttfb += benchmark.ttfb / 1e9;
										count++;
									}
								});
								if (count > 0) {
									ul /= count;
									dl /= count;
									ttfb /= count;
									benchmarks.push({
										location: loc,
										upload: convertSize(ul) + '/s',
										download: convertSize(dl) + '/s',
										ttfb: ttfb.toFixed(2) + ' s',
										data: data.benchmarks
									});
								} else {
									benchmarks.push({
										location: loc,
										upload: 'N/A',
										download: 'N/A',
										ttfb: 'N/A',
										data: data.benchmarks
									});
								}
								if (benchmarks.length == locations.length) {
									let upload = '<tr><td>Upload Speed</td>';
									let download = '<tr><td>Download Speed</td>';
									let ttfb = '<tr><td>TTFB</td>';
									let header = '<tr>';
									locations.forEach(l => {
										let benchmark = benchmarks.find(b => b.location == l);
										upload += '<td style="text-align:center">' + benchmark.upload + '</td>';
										download += '<td style="text-align:center">' + benchmark.download + '</td>';
										ttfb += '<td style="text-align:center">' + benchmark.ttfb + '</td>';
										header += '<th>' + l + '</th>';
									});
									header += '</tr>';
									upload += '</tr>';
									download += '</tr>';
									ttfb += '</tr>';
									table.children[1].innerHTML += upload + download + ttfb;
									list.children[0].innerHTML = header;
									let index = 0;
									while (true) {
										let row = '';
										let empty = true;
										locations.forEach(l => {
											let benchmark = benchmarks.find(b => b.location == l);
											if (index < benchmark.data.length) {
												row += '<td style="text-align:center">' +
													'<span class="benchmark-' + (benchmark.data[index].success == true ? 'pass">' : 'fail">') +
													new Date(benchmark.data[index].timestamp).toLocaleString() + '</span>' +
													(benchmark.data[index].success == true ? '' : '<br>' +
													benchmark.data[index].error) + '</td>';
												empty = false;
											} else {
												row += '<td></td>';
											}
										})
										if (empty) break;
										index++;
										list.children[0].innerHTML += '<tr>' + row + '</tr>';
									}
								}
							}
						})
						.catch(error => console.log(error));
					});
					table.classList.remove('hidden');
					list.classList.remove('hidden');
				}
			}
		})
		.catch(error => console.log(error));
	});
	document.getElementById('current-id').innerHTML = host.id;
	document.getElementById('current-key').innerHTML = host.publicKey;
	document.getElementById('current-address').innerHTML = host.netaddress;
	document.getElementById('current-location').innerHTML = getFlagEmoji(host.country);
	document.getElementById('current-first').innerHTML = new Date(host.firstSeen).toDateString();
	document.getElementById('current-last').innerHTML = host.lastSeen == '0001-01-01T00:00:00Z' ? 'N/A' : new Date(host.lastSeen).toDateString();
	document.getElementById('current-uptime').innerHTML = host.downtime + host.uptime == 0 ? '0%' : (host.uptime * 100 / (host.uptime + host.downtime)).toFixed(1) + '%';
	document.getElementById('current-version').innerHTML = host.settings.version == '' ? 'N/A' : host.settings.version;
	document.getElementById('current-accepting').innerHTML = host.settings.acceptingcontracts ? 'Yes' : 'No';
	document.getElementById('current-duration').innerHTML = host.settings.maxduration == 0 ? 'N/A' : blocksToTime(host.settings.maxduration);
	document.getElementById('current-contract-price').innerHTML = convertPrice(host.settings.contractprice);
	document.getElementById('current-storage-price').innerHTML = convertPricePerBlock(host.settings.storageprice);
	document.getElementById('current-collateral').innerHTML = convertPricePerBlock(host.settings.collateral);
	document.getElementById('current-upload-price').innerHTML = host.settings.uploadbandwidthprice == '0' ? '0 H/TB' : convertPrice(host.settings.uploadbandwidthprice + '0'.repeat(12)) + '/TB';
	document.getElementById('current-download-price').innerHTML = host.settings.downloadbandwidthprice == '0' ? '0 H/TB' : convertPrice(host.settings.downloadbandwidthprice + '0'.repeat(12)) + '/TB';
	document.getElementById('current-total').innerHTML = convertSize(host.settings.totalstorage);
	document.getElementById('current-remaining').innerHTML = convertSize(host.settings.remainingstorage);
	document.getElementById('host').classList.remove('hidden');
}

function blocksToTime(blocks) {
	if (blocks < 144 * 7) return (blocks / 144).toFixed(1) + ' days';
	if (blocks < 144 * 30) return (blocks / 144 / 7).toFixed(1) + ' weeks';
	return (blocks / 144 / 30).toFixed(1) + ' months';
}

function convertSize(size) {
	if (size < 1024) {
		return '' + size + ' B';
	}
	const sizes = ['KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
	let i = Math.floor(Math.log10(size) / 3);
	let s = (size / Math.pow(10, 3 * i)).toFixed(1);
	return s.slice(0, s.indexOf('.') + i + 1) + ' ' + sizes[i - 1];
}

function convertPrice(value) {
	if (value.length < 13) return value + ' H';
	if (value.length < 16) return value.slice(0, value.length - 12) + ' pS';
	if (value.length < 19) return value.slice(0, value.length - 15) + ' nS';
	if (value.length < 22) return value.slice(0, value.length - 18) + ' uS';
	if (value.length < 25) return value.slice(0, value.length - 21) + ' mS';
	if (value.length < 28) {
		let result = value.slice(0, value.length - 24);
		if (value[value.length-24] != '0') result += '.' + value[value.length-24];
		return result + ' SC';
	}
	if (value.length < 31) {
		let result = value.slice(0, value.length - 27);
		if (value[value.length-27] != '0') result += '.' + value[value.length-27];
		return result + ' KS';
	}
	let result = value.slice(0, value.length - 30);
	if (value[value.length-30] != '0') result += '.' + value[value.length-30];
	return result + ' MS';
}

function convertPricePerBlock(value) {
	if (value.length > 12) {
		value = value.slice(0, value.length - 12) + '.' + value.slice(value.length - 12);
	} else {
		value = '0.' + '0'.repeat(12 - value.length) + value;
	}
	let result = parseFloat(value) * 144 * 30;
	if (result < 1e-12) return (result * 1e24).toFixed(0) + ' H/TB/month';
	if (result < 1e-9) return (result * 1e24).toFixed(0) + ' pS/TB/month';
	if (result < 1e-6) return (result * 1e24).toFixed(0) + ' uS/TB/month';
	if (result < 1e-3) return (result * 1e24).toFixed(0) + ' mS/TB/month';
	if (result < 1) return result.toFixed(1) + ' SC/TB/month';
	if (result < 1e3) return result.toFixed(0) + ' SC/TB/month';
	if (result < 1e6) return (result / 1e3).toFixed(1) + ' KS/TB/month';
	return (result / 1e6).toFixed(1) + ' MS/TB/month';
}

function getFlagEmoji(country) {
	if (country == '') return N/AbortController;
	const codePoints = country
	  .toUpperCase()
	  .split('')
	  .map(char =>  127397 + char.charCodeAt());
	return String.fromCodePoint(...codePoints);
  }
