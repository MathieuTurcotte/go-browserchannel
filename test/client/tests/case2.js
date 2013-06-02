/*
 * Copyright (c) 2013 Mathieu Turcotte
 * Licensed under the MIT license.
 */

goog.provide('tests.Handler2');

goog.require('goog.Timer');
goog.require('goog.net.BrowserChannel.Handler');



/**
 * A test handler which sends messages to the server and expects acknowledgment
 * responses from it within a given delay. The handler will fail if the server
 * doesn't respond to a request within the expected delay. The handler will
 * close the browser channel upon completion of the test.
 * @extends {goog.net.BrowserChannel.Handler}
 * @constructor
 */
tests.Handler2 = function() {
    goog.base(this);

    /**
     * @private {number}
     */
    this.numMapsSent_ = 0;

    /**
     * @private {number}
     */
    this.nextMapId_ = 0;

    /**
     * @private {number}
     */
    this.expectedMapId_ = -1;
};
goog.inherits(tests.Handler2, goog.net.BrowserChannel.Handler);


/**
 * Maximum delay to wait for an array from the server. If the delay is
 * exceeded, the handler will signal a failure to the phantom runner.
 * @type {number}
 * @const
 */
tests.Handler2.TIMEOUT_DELAY = 5 * 1000;


/**
 * Maximum delay to wait for an array from the server. If the delay is
 * exceeded, the handler will signal a failure to the phantom runner.
 * @type {number}
 * @const
 */
tests.Handler2.SEND_INTERVAL = 15 * 1000;


/**
 * @type {number}
 * @const
 */
tests.Handler2.NUM_MAP_TO_SEND = 10;


/** @override */
tests.Handler2.prototype.channelOpened = function(channel) {
    channel.sendMap({id: '2'});
    this.sendNextArray_(channel);
};


/** @override */
tests.Handler2.prototype.channelHandleArray = function(channel, array) {
    var id = array[0];

    if (id != this.expectedMapId_) {
        window.callPhantom({type: 'done', success: false});
        channel.disconnect();
    } else if (this.numMapsSent_ == tests.Handler2.NUM_MAP_TO_SEND) {
        channel.disconnect();
    } else {
        this.expectedMapId_ = -1;
        goog.Timer.callOnce(goog.partial(this.sendNextArray_, channel),
            tests.Handler2.SEND_INTERVAL, this);
    }
};


/** @override */
tests.Handler2.prototype.channelError = function(channel, error) {
    switch (error) {
        case goog.net.BrowserChannel.Error.STOP:
            break;
        default:
            window.callPhantom({type: 'done', success: false});
    }
};


/** @override */
tests.Handler2.prototype.channelClosed = function(channel) {
    window.callPhantom({type: 'done', success: true});
};


/** @override */
tests.Handler2.prototype.badMapError = function(browserChannel, map) {
    window.callPhantom({type: 'done', success: false});
};


/**
 * @param {!goog.net.BrowserChannel} channel The browser channel.
 * @private
 */
tests.Handler2.prototype.sendNextArray_ = function(channel) {
    this.numMapsSent_++;
    this.expectedMapId_ = this.nextMapId_;
    channel.sendMap({payload: this.nextMapId_++});
};


/**
 * Handles the timeout triggered if no arrays were received within the
 * expected delay from the server.
 * @private
 */
tests.Handler2.prototype.onTimeout_ = function() {
    window.callPhantom({type: 'done', success: false});
};
