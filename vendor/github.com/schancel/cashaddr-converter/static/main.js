/* jshint esversion: 6 */

// QR Codes: https://davidshimjs.github.io/qrcodejs/

;(() => {

document.addEventListener('DOMContentLoaded', () => {
	const dispatch = ui.initialize({
		container: ui.element('container'),
		state: {
			scene: 'loading',
			form: {
				address: ''
			},
			address: {},
			tab: 'cashaddr',
			error: ''
		},
		events: {
			'submit': {
				'click': ['submit-address']
			},
			'start': {
				'click': ['dismiss-error']
			},
			'click-tab': {
				'click': ['cashaddr-tab', 'copay-tab', 'legacy-tab']
			},
			'copy': {
				'click': ['copy-cashaddr', 'copy-copay', 'copy-legacy']
			},
			'change-address': {
				'change': ['address']
			}
		},

		scenes, integrate, react
	});

	dispatch('start');
});

function integrate(state, action, payload) {
	const {scene, form, address, tab, error} = state;
	switch (action) {
	case 'start':
		return {
			scene: 'form',
			form, address, tab, error
		};
	case 'change-address':
		return {
			form: ui.merge(state.form, {[payload.name]: payload.value}),
			scene, address, tab, error
		};
	case 'submit':
		return {
			scene: 'loading',
			form, address, tab, error
		};
	case 'convert-success':
		return {
			scene: 'address',
			address: payload,
			form, tab, error
		};
	case 'convert-error':
		return {
			scene: 'error',
			error: payload.error,
			form, tab, address
		};
	case 'click-tab':
		return {
			tab: payload.name,
			scene, form, address, error
		};
	}
	return state;
}

const react = (old, action, payload) => (state, render, dispatch) => {
	if (old.scene !== state.scene) render();

	switch (action) {
	case 'submit':
		fetch(`/convert?address=${encodeURIComponent(state.form.address)}`)
			.then((response) => {
				if (response.ok) {
					return response.json();
				} else {
					return response.text().then((message) => { throw message; });
				}
			})
			.then(
				(addresses) => dispatch('convert-success', addresses),
				(message) => dispatch(`convert-error`, {error: message})
			);
		break;

	case 'copy':
		switch (payload.name) {
		case 'copy-cashaddr':
			ui.element('cashaddr-address').select();
			break;
		case 'copy-copay':
			ui.element('copay-address').select();
			break;
		case 'copy-legacy':
			ui.element('legacy-address').select();
		}
		if (document.execCommand('copy')) {
			dispatch('copy-success');
		} else {
			dispatch('copy-error');
		}
		break;

	case 'click-tab':
	case 'convert-success':
		render();
		new QRCode(ui.element(state.tab), {
			text: state.address[state.tab],
			mode: 1,
			width: 256,
			height: 256,
			correctLevel: QRCode.CorrectLevel.L,
		});
	}
};

const scenes = {
	loading: (scenes, state) => `
		${scenes.form(scenes, state)}
		<div class="sk-folding-cube">
			<div class="sk-cube1 sk-cube"></div>
			<div class="sk-cube2 sk-cube"></div>
			<div class="sk-cube4 sk-cube"></div>
			<div class="sk-cube3 sk-cube"></div>
		</div>
	`,


	form: (scenes, state) => `
		<form id="address-form">
			<input type="text"
				id="address"
				name="address"
				autofocus
				autocomplete="off"
				placeholder="qr2z7dusk64qn960h9vspf2ezewl0pla9gcpnk35f0"
				value="${state.form.address}"
				maxlength="64" />
			<input type="submit" id="submit-address" value="Convert">
		</form>
	`,

	address: (scenes, state) => `
		${scenes.form(scenes, state)}
		<div id="qr-codes">
			<div id="tabs">
				<button class="tab${state.tab === 'cashaddr' ? ' selected-tab' : ''}"
					id="cashaddr-tab" name="cashaddr">
					CashAddr
				</button>
				<button class="tab${state.tab === 'copay' ? ' selected-tab' : ''}"
					id="copay-tab" name="copay">
					Copay
				</button>
				<button class="tab${state.tab === 'legacy' ? ' selected-tab' : ''}"
					id="legacy-tab" name="legacy">
					Legacy
				</button>
			</div>
			<div class="qr-card${state.tab === 'cashaddr' ? ' selected-tab' : ''}">
				<div class="qr-address">
					<input readonly type="text"
						id="cashaddr-address"
						name="cashaddr"
						value="${state.address.cashaddr}" />
					<button id="copy-cashaddr"
						title="copy cashaddr to clipboard"
						name="copy-cashaddr">
						<span class="ion-clipboard"></span>
					</button>
				</div>
				<div id="cashaddr" class="qr-code"></div>
			</div>
			<div class="qr-card${state.tab === 'copay' ? ' selected-tab' : ''}">
				<div class="qr-address">
					<input readonly type="text"
						id="copay-address"
						name="copay"
						value="${state.address.copay}" />
					<button id="copy-copay"
						title="copy copay to clipboard"
						name="copy-copay">
						<span class="ion-clipboard"></span>
					</button>
				</div>
				<div id="copay" class="qr-code"></div>
			</div>
			<div class="qr-card${state.tab === 'legacy' ? ' selected-tab' : ''}">
				<div class="qr-address">
					<input readonly type="text"
						id="legacy-address"
						name="legacy"
						value="${state.address.legacy}" />
					<button id="copy-legacy"
						title="copy legacy address to clipboard"
						name="copy-legacy">
						<span class="ion-clipboard"></span>
					</button>
				</div>
				<div id="legacy" class="qr-code"></div>
			</div>
		</div>
	`,

	error: (scenes, state) => `
		${scenes.form(scenes, state)}
		<div id="error-container">
			<div id="error">
				<div>
					<span class="ion-alert-circled"></span>
					${state.error}
				</div>
				<div id="dismiss-error" class="ion-close"></div>
			</div>
		</div>
	`,
};

})();

;(() => {

const defer = (fn) => setTimeout(fn, 0);
const delay = (duration) => (value) => new Promise((resolve) => setTimeout(() => resolve(value), duration));
const merge = (...args) => Object.assign({}, ...args);
const dedupe = (array) => keys(array.reduce((out, string) => (out[string] = true) && out, {}));
const keys = Object.keys;
const flatten = (array) => array.reduce((out, ary) => out.concat(ary), []);
const values = (object) => keys(object).map(key => object[key]);
const unzip = (object) => keys(object).map(key => [key, object[key]]);
const first = (array) => array[0];
const once = (fn, value, done = false) => (...args) => {
	if (done) return value;
	done = true;
	return (value = fn(...args));
};

const element = document.getElementById.bind(document);

const initialize = ({container, state, events, scenes, integrate, react}) => {
	const dispatch = (action, payload) => {
		if (window.DEBUG) console.log(`[${action}]`, payload);
		const postintegrate = react(state, action, payload);
		state = integrate(state, action, payload);
		const render = once(() => {
			container.innerHTML = scenes[state.scene](scenes, state);
		});
		postintegrate(state, render, (...args) => defer(() => dispatch(...args)));
	};

	dedupe(flatten(values(events).map(unzip)).map(first)).forEach(event => {
		const ids = {};
		unzip(events).forEach(([action, def]) => {
			if (!def[event]) return;
			def[event].forEach(id => {
				ids[id] = ids[id] || [];
				ids[id].push(action);
			});
		});

		container.addEventListener(event, (e) => {
			const {id, name, value} = e.target;
			if (!ids[id]) return;
			e.preventDefault();
			ids[id].forEach(action => dispatch(action, {name, value}));
		});
	});

	return dispatch;
};

window.ui = {
	defer, delay, once,
	keys, values, merge, unzip,
	dedupe, first, flatten,
	element, initialize,
};

})();