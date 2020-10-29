import {id as pluginId} from './manifest';
import Client from './client';

let activityFunc;
let lastActivityTime = 0;
const activityTimeout = 1 * 60 * 1000; // 1 min

export default class Plugin {
    // eslint-disable-next-line no-unused-vars
    public initialize(registry, store) {
        // @see https://developers.mattermost.com/extend/plugins/webapp/reference/
        console.log('Loading plugin');
        activityFunc = () => {
            console.log('Loading clicking');
            const now = new Date().getTime();
            console.log(now - lastActivityTime > activityTimeout);
            if (now - lastActivityTime > activityTimeout) {
                console.log('REQUEST');
                try {
                    const resp = (new Client()).getConnected();
                    console.log(resp);
                } catch (e) {
                    console.log('ERROR', e)
                }
                lastActivityTime = now;
            }
        };

        document.addEventListener('click', activityFunc);
    }

    public deinitialize() {
        document.removeEventListener('click', activityFunc);
    }
}

window.registerPlugin(pluginId, new Plugin());
